package heifthumb

import (
	"encoding/binary"
	"io"
	"math"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// maxMetaBufferBytes caps how much of the top-level meta box ReadMeta keeps in
// memory. Real HEIC/AVIF meta boxes are a few KiB (tens of KiB for large grids);
// anything bigger is parsed child-by-child with ReadAt instead.
const maxMetaBufferBytes = 1 << 20

// Meta holds the bytes of a file's top-level meta box, read once with a
// bounded top-level box-header walk, so the irot/imir lookup, the embedded
// JPEG lookup and the coded-image decoder can share them.
type Meta struct {
	off   int64  // file offset of the meta box header
	box   []byte // whole meta box (header included); nil when over the cap
	start int64  // meta payload start (after box header and full-box fields)
	end   int64  // meta box end
}

// ReadMeta walks the top-level box headers with ReadAt and reads only the meta
// box. The returned Meta always records where meta lives; its bytes are kept
// only when the box is at most 1 MiB.
func ReadMeta(r io.ReaderAt, size int64) (*Meta, error) {
	if size <= 0 {
		return nil, errs.Corrupt
	}
	var off int64
	var hdr [16]byte
	for i := 0; off+8 <= size && i < maxTopBoxes; i++ {
		typ, hl, bs, err := readBoxHeader(r, size, off, size, hdr[:])
		if err != nil {
			return nil, err
		}
		if typ == [4]byte{'m', 'e', 't', 'a'} {
			m := &Meta{off: off, start: off + hl + 4, end: off + bs}
			if m.start > m.end {
				return nil, errs.Corrupt
			}
			if bs <= maxMetaBufferBytes {
				b := make([]byte, bs)
				if !readFullAt(r, b, off) {
					return nil, errs.Corrupt
				}
				m.box = b
			}
			return m, nil
		}
		off += bs
	}
	return nil, errs.Unsup("thumbnail", "heif: no meta box")
}

// ReaderAt returns r with reads that fall inside the buffered meta box served
// from memory, so later meta parsing (JPEGPreview, CodedPrimaryYCbCr) does not
// read those bytes again. Reads outside meta go to r.
func (m *Meta) ReaderAt(r io.ReaderAt) io.ReaderAt {
	if m == nil || m.box == nil {
		return r
	}
	return metaReaderAt{r: r, off: m.off, b: m.box}
}

type metaReaderAt struct {
	r   io.ReaderAt
	off int64
	b   []byte
}

func (m metaReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= m.off && off+int64(len(p)) <= m.off+int64(len(m.b)) {
		return copy(p, m.b[off-m.off:]), nil
	}
	return m.r.ReadAt(p, off)
}

// Transform is the primary item's display transform from its irot and imir
// properties, normalized to one form: first rotate counter-clockwise by
// Rotation, then mirror. The properties themselves are composed in ipma
// association order, as HEIF and libheif define it, so a mirror associated
// before a quarter-turn rotation is reported as the rotation followed by a
// mirror across the other axis.
type Transform struct {
	// Rotation is 0, 90, 180 or 270 degrees counter-clockwise (irot × 90).
	Rotation int
	// Mirror reports whether an imir property is present.
	Mirror bool
	// MirrorAxis is the imir axis bit: 0 flips top-to-bottom (vertical flip),
	// 1 flips left-to-right (horizontal flip). This matches ffmpeg's mov
	// demuxer (axis 0 → vflip, axis 1 → hflip) and libavif.
	MirrorAxis uint8
}

// PrimaryTransform returns the primary item's irot/imir transform. ok is false
// when the primary item has neither property (or meta cannot be parsed).
func (m *Meta) PrimaryTransform(r io.ReaderAt, size int64) (Transform, bool) {
	t, found, err := m.PrimaryTransformErr(r, size)
	return t, found && err == nil
}

// PrimaryTransformErr is PrimaryTransform that tells a file without irot and
// imir (found false, nil error) from one whose pitm, iprp, ipco or ipma
// cannot be read (errs.Unsup), so callers can fall back instead of assuming
// no transform.
func (m *Meta) PrimaryTransformErr(r io.ReaderAt, size int64) (Transform, bool, error) {
	if m == nil {
		return Transform{}, false, errs.Unsup("heif transform", "no meta box")
	}
	if m.box != nil {
		return parsePrimaryTransform(m.box)
	}
	// Oversized meta: read only pitm and iprp, each capped like the buffer.
	var pitm, iprp []byte
	var hdr [16]byte
	for off := m.start; off+8 <= m.end; {
		typ, _, bs, err := readBoxHeader(r, size, off, m.end, hdr[:])
		if err != nil {
			break
		}
		switch string(typ[:]) {
		case "pitm", "iprp":
			if bs <= maxMetaBufferBytes {
				b := make([]byte, bs)
				if readFullAt(r, b, off) {
					if typ[0] == 'p' {
						pitm = b
					} else {
						iprp = b
					}
				}
			}
		}
		off += bs
	}
	return primaryTransformFrom(pitm, iprp)
}

// PrimaryDisplayRotation reads the primary image item's irot property and returns
// the counter-clockwise rotation in degrees (0, 90, 180, 270) to apply for display,
// matching thumb.rotateImage / ffmpeg autorotate. Use ReadMeta and
// Meta.PrimaryTransform to also get imir.
func PrimaryDisplayRotation(r io.ReaderAt, size int64) (int, bool) {
	m, err := ReadMeta(r, size)
	if err != nil {
		return 0, false
	}
	t, ok := m.PrimaryTransform(r, size)
	if !ok {
		return 0, false
	}
	return t.Rotation, true
}

// irotDegrees maps HEIF irot (0–3) to counter-clockwise degrees for display.
func irotDegrees(irot uint8) int {
	switch irot & 0x03 {
	case 1:
		return 90
	case 2:
		return 180
	case 3:
		return 270
	default:
		return 0
	}
}

func parsePrimaryTransform(data []byte) (Transform, bool, error) {
	meta := findBMFFBox(data, "meta")
	if meta == nil {
		return Transform{}, false, errs.Unsup("heif transform", "no meta box")
	}
	return primaryTransformFrom(findBMFFChildBox(meta, "pitm"), findBMFFChildBox(meta, "iprp"))
}

func primaryTransformFrom(pitm, iprp []byte) (Transform, bool, error) {
	primaryID := parsePITMItemID(pitm)
	if primaryID == 0 || iprp == nil {
		return Transform{}, false, errs.Unsup("heif transform", "no primary item properties")
	}
	ipco := findBMFFChildBox(iprp, "ipco")
	ipma := findBMFFChildBox(iprp, "ipma")
	if ipco == nil || ipma == nil {
		return Transform{}, false, errs.Unsup("heif transform", "no ipco or ipma")
	}
	properties := parseIPCOProperties(ipco)
	indices, err := parseIPMAPropertyIndices(ipma, primaryID)
	if err != nil {
		return Transform{}, false, err
	}
	var t Transform
	found := false
	// HEIF applies transformative properties in association order, so each
	// irot or imir is composed onto the transform so far (libheif does the
	// same). The result is kept in the rotate-then-mirror form Transform
	// documents.
	for _, idx := range indices {
		if idx == 0 || int(idx) > len(properties) {
			continue
		}
		switch prop := properties[idx-1]; prop.typ {
		case "irot":
			t = t.thenRotate(irotDegrees(prop.val))
			found = true
		case "imir":
			t = t.thenMirror(prop.val & 0x01)
			found = true
		}
	}
	return t, found, nil
}

// thenRotate returns t followed by a counter-clockwise rotation of deg
// degrees, in rotate-then-mirror form. A quarter turn after a mirror equals
// the same quarter turn before the mirror across the other axis; a half turn
// keeps the axis.
func (t Transform) thenRotate(deg int) Transform {
	t.Rotation = (t.Rotation + deg) % 360
	if t.Mirror && (deg == 90 || deg == 270) {
		t.MirrorAxis ^= 1
	}
	return t
}

// thenMirror returns t followed by a mirror across axis (0 top-to-bottom,
// 1 left-to-right), in rotate-then-mirror form. Two mirrors across the same
// axis cancel; across different axes they make a half turn.
func (t Transform) thenMirror(axis uint8) Transform {
	switch {
	case !t.Mirror:
		t.Mirror, t.MirrorAxis = true, axis
	case t.MirrorAxis == axis:
		t.Mirror, t.MirrorAxis = false, 0
	default:
		t.Mirror, t.MirrorAxis = false, 0
		t.Rotation = (t.Rotation + 180) % 360
	}
	return t
}

const maxBMFFRecursionDepth = 32

func findBMFFBox(data []byte, typ string) []byte {
	return findBMFFBoxRange(data, 0, len(data), typ, 0)
}

func findBMFFBoxRange(data []byte, start, end int, typ string, depth int) []byte {
	if depth > maxBMFFRecursionDepth {
		return nil
	}
	pos := start
	for pos+8 <= end {
		size, header, ok := bmffBoxHeader(data, pos, end)
		if !ok || size < header {
			break
		}
		boxEnd := pos + size
		if boxEnd > end || boxEnd < pos {
			break
		}
		if string(data[pos+4:pos+8]) == typ {
			return data[pos:boxEnd]
		}
		if isBMFFContainerType(data[pos+4 : pos+8]) {
			if child := findBMFFBoxRange(data, pos+header, boxEnd, typ, depth+1); child != nil {
				return child
			}
		}
		pos = boxEnd
	}
	return nil
}

func findBMFFChildBox(container []byte, typ string) []byte {
	payload := bmffBoxPayload(container)
	if payload == nil {
		return nil
	}
	return findBMFFBoxRange(payload, 0, len(payload), typ, 0)
}

func isBMFFContainerType(typ []byte) bool {
	switch string(typ) {
	case "meta", "iprp", "ipco", "moov", "trak", "mdia", "minf", "stbl", "dinf":
		return true
	default:
		return false
	}
}

func bmffBoxHeader(data []byte, pos, end int) (size int, header int, ok bool) {
	if pos+8 > end {
		return 0, 0, false
	}
	size32 := binary.BigEndian.Uint32(data[pos : pos+4])
	if size32 == 0 {
		return 0, 0, false
	}
	if size32 == 1 {
		if pos+16 > end {
			return 0, 0, false
		}
		size64 := binary.BigEndian.Uint64(data[pos+8 : pos+16])
		if size64 < 16 || size64 > math.MaxInt32 {
			return 0, 0, false
		}
		return int(size64), 16, true
	}
	if size32 < 8 {
		return 0, 0, false
	}
	return int(size32), 8, true
}

func bmffBoxPayload(box []byte) []byte {
	if len(box) < 8 {
		return nil
	}
	typ := string(box[4:8])
	header := bmffBoxHeaderSize(box)
	if isBMFFFullBoxType(typ) {
		header += 4
	}
	if header > len(box) {
		return nil
	}
	return box[header:]
}

func bmffBoxHeaderSize(box []byte) int {
	if len(box) >= 8 && binary.BigEndian.Uint32(box[0:4]) == 1 {
		return 16
	}
	return 8
}

func bmffFullBoxVersionFlags(box []byte) (version byte, flags byte, ok bool) {
	if len(box) < 8 || !isBMFFFullBoxType(string(box[4:8])) {
		return 0, 0, false
	}
	off := bmffBoxHeaderSize(box)
	if len(box) < off+4 {
		return 0, 0, false
	}
	version = box[off]
	flags = box[off+3]
	return version, flags, true
}

func isBMFFFullBoxType(typ string) bool {
	switch typ {
	case "meta", "pitm", "ispe", "ipma", "hdlr", "iloc", "iinf", "infe":
		return true
	default:
		return false
	}
}

func parsePITMItemID(pitm []byte) uint32 {
	payload := bmffBoxPayload(pitm)
	version, _, ok := bmffFullBoxVersionFlags(pitm)
	if !ok || payload == nil {
		return 0
	}
	switch version {
	case 0:
		if len(payload) < 2 {
			return 0
		}
		return uint32(binary.BigEndian.Uint16(payload[0:2]))
	default:
		if len(payload) < 4 {
			return 0
		}
		return binary.BigEndian.Uint32(payload[0:4])
	}
}

type ipcoProperty struct {
	typ string
	val uint8 // irot angle (low 2 bits) or imir axis (low bit)
}

func parseIPCOProperties(ipco []byte) []ipcoProperty {
	payload := bmffBoxPayload(ipco)
	if payload == nil {
		return nil
	}
	var props []ipcoProperty
	pos := 0
	for pos+8 <= len(payload) {
		size, header, ok := bmffBoxHeader(payload, pos, len(payload))
		if !ok || size < header || pos+size > len(payload) {
			break
		}
		child := payload[pos : pos+size]
		prop := ipcoProperty{typ: string(child[4:8])}
		switch prop.typ {
		case "irot":
			if p := bmffBoxPayload(child); len(p) >= 1 {
				prop.val = p[0] & 0x03
			}
		case "imir":
			if p := bmffBoxPayload(child); len(p) >= 1 {
				prop.val = p[0] & 0x01
			}
		}
		props = append(props, prop)
		pos += size
	}
	return props
}

// errMalformedIPMA reports an ipma box too short for its entry count or for
// the entries it declares.
var errMalformedIPMA = errs.Unsup("heif transform", "malformed ipma")

// parseIPMAPropertyIndices returns the property indices associated with
// itemID, in association order. An item with no entry returns nil and a nil
// error; an ipma box that is too short for its entry count or entries
// returns errMalformedIPMA, so callers do not read a broken box as "no
// properties".
func parseIPMAPropertyIndices(ipma []byte, itemID uint32) ([]uint16, error) {
	payload := bmffBoxPayload(ipma)
	version, flags, ok := bmffFullBoxVersionFlags(ipma)
	if !ok || payload == nil || len(payload) < 4 {
		return nil, errMalformedIPMA
	}
	wide := flags&0x01 != 0
	entrySize := 1
	if wide {
		entrySize = 2
	}
	entryCount := int(binary.BigEndian.Uint32(payload[0:4]))
	pos := 4
	for i := 0; i < entryCount; i++ {
		var id uint32
		switch version {
		case 0:
			if pos+2 > len(payload) {
				return nil, errMalformedIPMA
			}
			id = uint32(binary.BigEndian.Uint16(payload[pos : pos+2]))
			pos += 2
		default:
			if pos+4 > len(payload) {
				return nil, errMalformedIPMA
			}
			id = binary.BigEndian.Uint32(payload[pos : pos+4])
			pos += 4
		}
		if pos >= len(payload) {
			return nil, errMalformedIPMA
		}
		count := int(payload[pos])
		pos++
		if pos+count*entrySize > len(payload) {
			return nil, errMalformedIPMA
		}
		if id != itemID {
			pos += count * entrySize
			continue
		}
		indices := make([]uint16, count)
		for j := 0; j < count; j++ {
			if wide {
				indices[j] = binary.BigEndian.Uint16(payload[pos+2*j:]) & 0x7fff
			} else {
				indices[j] = uint16(payload[pos+j] & 0x7f)
			}
		}
		return indices, nil
	}
	return nil, nil
}
