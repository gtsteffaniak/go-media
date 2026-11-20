package matroska

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// ebmlType is the data type of a Matroska element. The ebmlTop bit is set on the
// top-level Segment children (Info, Tracks, Cluster and so on).
type ebmlType uint8

// Element data types, plus the ebmlTop flag.
const (
	ebmlMaster ebmlType = iota
	ebmlInt
	ebmlUint
	ebmlDate
	ebmlFloat
	ebmlBinary
	ebmlString
	ebmlBlock

	ebmlTop      ebmlType = 0x80
	ebmlKindMask ebmlType = 0x7F
)

// Element IDs of the Info and Tracks fields readHead stores. idInfo and idTracks
// are declared in probe.go.
const (
	eidTimecodeScale        = 0x2AD7B1
	eidDuration             = 0x4489
	eidTrackEntry           = 0xAE
	eidTrackNumber          = 0xD7
	eidTrackType            = 0x83
	eidCodecID              = 0x86
	eidCodecPrivate         = 0x63A2
	eidLanguage             = 0x22B59C
	eidName                 = 0x536E
	eidDefaultDuration      = 0x23E383
	eidCodecDelay           = 0x56AA
	eidTrackTimestampScale  = 0x23314F
	eidContentEncodings     = 0x6D80
	eidContentEncoding      = 0x6240
	eidContentEncodingScope = 0x5032
	eidContentEncodingType  = 0x5033
	eidContentCompression   = 0x5034
	eidContentCompAlgo      = 0x4254
	eidContentCompSettings  = 0x4255
	eidVideo                = 0xE0
	eidPixelWidth           = 0xB0
	eidPixelHeight          = 0xBA
	eidAudio                = 0xE1
	eidSamplingFrequency    = 0xB5
	eidChannels             = 0x9F
	eidBitDepth             = 0x6264
)

// Decoder limits.
const (
	// ebmlSizeUnknown is the size readEBMLSize reports for an unknown-size element.
	ebmlSizeUnknown = 1<<56 - 1
	// ebmlMaxDepth bounds master element nesting so crafted input cannot recurse
	// without limit.
	ebmlMaxDepth = 1024
	// ebmlMaxLeafSize is the largest Binary, String or Block element accepted.
	ebmlMaxLeafSize = 128 << 20
)

// Decode errors. All of them unwrap to errs.Corrupt.
var (
	errEBMLTruncated   = fmt.Errorf("%w: ebml: element truncated", errs.Corrupt)
	errEBMLUnknownID   = fmt.Errorf("%w: ebml: unknown element ID", errs.Corrupt)
	errEBMLElementSize = fmt.Errorf("%w: ebml: invalid element size", errs.Corrupt)
	errEBMLFloatSize   = fmt.Errorf("%w: ebml: float element is not 4 or 8 bytes", errs.Corrupt)
	errEBMLTooLarge    = fmt.Errorf("%w: ebml: element too large", errs.Corrupt)
	errEBMLTooDeep     = fmt.Errorf("%w: ebml: elements nested too deep", errs.Corrupt)
	errEBMLLacing      = fmt.Errorf("%w: ebml: invalid block lacing", errs.Corrupt)
)

// ebmlElement describes one schema entry: its ID with the length marker, the ID
// length in bytes, and its data type.
type ebmlElement struct {
	id    uint32
	idLen int
	typ   ebmlType
}

// kind returns the element's data type without the ebmlTop flag.
func (e ebmlElement) kind() ebmlType { return e.typ & ebmlKindMask }

// top reports whether the element is a top-level Segment child.
func (e ebmlElement) top() bool { return e.typ&ebmlTop != 0 }

// ebmlByValue indexes ebmlElements by the ID value with its length marker
// removed, which is the value readEBMLVint returns.
var ebmlByValue = buildEBMLIndex()

// buildEBMLIndex returns ebmlElements keyed by marker-free ID value.
func buildEBMLIndex() map[uint32]ebmlElement {
	m := make(map[uint32]ebmlElement, len(ebmlElements))
	for id, typ := range ebmlElements {
		n := 4
		for n > 1 && id>>(8*(n-1)) == 0 {
			n--
		}
		value := id &^ (1 << (8*n - n))
		m[value] = ebmlElement{id: id, idLen: n, typ: typ}
	}
	return m
}

// ebmlSink receives decoded children of one master element. A nil sink, or a nil
// child from ebmlChild, means the element is decoded and checked but not stored.
type ebmlSink interface {
	// ebmlChild returns the sink for the master child with the given ID.
	ebmlChild(id uint32) ebmlSink
	// ebmlLeaf stores the raw payload of a leaf child. The payload has already
	// been checked against the element's data type.
	ebmlLeaf(id uint32, data []byte)
}

// ebmlHeader is an element header that a child level handed back to its parent
// to be read again (see readLevel).
type ebmlHeader struct {
	el      ebmlElement
	size    uint64
	sizeLen int
}

// ebmlDecoder decodes one buffered EBML element into an ebmlSink. It follows the
// rules of the ebml-go Unmarshal it replaced, so the same headers decode to the
// same values and the same malformed headers fail.
type ebmlDecoder struct {
	buf []byte
	cur int
}

// decodeEBML decodes the elements in buf into sink. buf normally holds exactly
// one element, such as Info or Tracks.
func decodeEBML(buf []byte, sink ebmlSink) error {
	d := ebmlDecoder{buf: buf}
	_, err := d.readLevel(len(buf), ebmlSizeUnknown, 0, ebmlElement{}, sink, 0)
	return err
}

// readEBMLVint reads one EBML variable-length integer at d.cur, staying before
// end. It returns the value with its length marker removed and the number of
// bytes read. A first byte of 0x00 reads as a one-byte zero. nb is 0 when no
// byte was available.
func (d *ebmlDecoder) readEBMLVint(end int) (value uint64, nb int, err error) {
	if d.cur >= end {
		return 0, 0, errEBMLTruncated
	}
	b := d.buf[d.cur]
	d.cur++
	n := 1
	if b != 0 {
		n = int(leadingZeros8(b)) + 1
		value = uint64(b) & (0xFF >> n)
	}
	for i := 1; i < n; i++ {
		if d.cur >= end {
			return 0, i, errEBMLTruncated
		}
		value = value<<8 | uint64(d.buf[d.cur])
		d.cur++
	}
	return value, n, nil
}

// leadingZeros8 returns the number of leading zero bits in b.
func leadingZeros8(b byte) int {
	n := 0
	for b&0x80 == 0 && n < 8 {
		b <<= 1
		n++
	}
	return n
}

// readEBMLSize reads an element data size. A size whose value bits are all ones
// is reported as ebmlSizeUnknown.
func (d *ebmlDecoder) readEBMLSize(end int) (size uint64, nb int, err error) {
	v, nb, err := d.readEBMLVint(end)
	if err != nil {
		return 0, nb, err
	}
	if v == math.MaxUint64>>(64-7*nb) {
		return ebmlSizeUnknown, nb, nil
	}
	return v, nb, nil
}

// reencodedSizeLen returns the length of a data size written back with the
// original length nb. Unknown sizes are always written as 8 bytes.
func reencodedSizeLen(size uint64, nb int) int {
	if size == ebmlSizeUnknown {
		return 8
	}
	return nb
}

// readLevel decodes the children of one master element and returns at the end
// of its data. end is where the bytes available to this level stop, n is the
// element's declared size (ebmlSizeUnknown for the outermost level and for
// unknown-size elements), pos is the logical stream position of the first
// child, and self is the element being read.
//
// A child header that ends this level is returned to the caller, which reads it
// again as one of its own children. That happens for a top-level element (such
// as Cluster) inside another top-level element, and for an EBML header inside an
// unknown-size element. pos is a running total of declared sizes, so after such
// a hand-back it runs ahead of d.cur; the size checks use pos, as ebml-go did.
func (d *ebmlDecoder) readLevel(end int, n, pos uint64, self ebmlElement, sink ebmlSink, depth int) (*ebmlHeader, error) {
	if depth > ebmlMaxDepth {
		return nil, errEBMLTooDeep
	}
	pos0 := pos
	var pending *ebmlHeader
	for {
		var h ebmlHeader
		var hdrLen uint64
		if pending != nil {
			h = *pending
			pending = nil
			hdrLen = uint64(h.el.idLen + h.sizeLen)
		} else {
			value, nb, err := d.readEBMLVint(end)
			if err != nil {
				if nb == 0 {
					return nil, nil
				}
				return nil, err
			}
			el, ok := ebmlByValue[uint32(value)]
			if !ok {
				return nil, errEBMLUnknownID
			}
			size, sb, err := d.readEBMLSize(end)
			if err != nil {
				return nil, err
			}
			h = ebmlHeader{el: el, size: size, sizeLen: sb}
			hdrLen = uint64(nb + sb)
		}
		if n != ebmlSizeUnknown && pos+hdrLen+h.size > pos0+n {
			return nil, errEBMLElementSize
		}
		if h.size == ebmlSizeUnknown && h.el.kind() != ebmlMaster {
			return nil, errEBMLElementSize
		}

		if h.el.kind() == ebmlMaster {
			if (h.el.top() && self.top()) || (h.el.id == idEBMLHeader && self.id != 0 && n == ebmlSizeUnknown) {
				h.sizeLen = reencodedSizeLen(h.size, h.sizeLen)
				return &h, nil
			}
			var child ebmlSink
			if sink != nil {
				child = sink.ebmlChild(h.el.id)
			}
			childEnd := end
			if h.size != ebmlSizeUnknown && h.size < uint64(end-d.cur) {
				childEnd = d.cur + int(h.size)
			}
			back, err := d.readLevel(childEnd, h.size, pos+hdrLen, h.el, child, depth+1)
			if err != nil {
				return nil, err
			}
			pending = back
		} else {
			data, err := d.readLeaf(h.el.kind(), h.size, end)
			if err != nil {
				return nil, err
			}
			if sink != nil && data != nil {
				sink.ebmlLeaf(h.el.id, data)
			}
		}
		pos += hdrLen + h.size
	}
}

// readLeaf checks and consumes the payload of a leaf element of the given type
// and size. It returns the payload for scalar, string and binary elements and
// nil for blocks, which are only checked.
func (d *ebmlDecoder) readLeaf(kind ebmlType, size uint64, end int) ([]byte, error) {
	switch kind {
	case ebmlInt, ebmlUint, ebmlDate:
		if size > 8 {
			return nil, errEBMLElementSize
		}
	case ebmlFloat:
		if size != 4 && size != 8 {
			return nil, errEBMLFloatSize
		}
	case ebmlBinary, ebmlString:
		if size > ebmlMaxLeafSize {
			return nil, errEBMLTooLarge
		}
	case ebmlBlock:
		if size > ebmlMaxLeafSize {
			return nil, errEBMLTooLarge
		}
		return nil, d.checkBlock(int64(size), end)
	}
	if size > uint64(end-d.cur) {
		return nil, errEBMLTruncated
	}
	start := d.cur
	d.cur += int(size)
	return d.buf[start:d.cur:d.cur], nil
}

// checkBlock checks a Block or SimpleBlock payload of n bytes: its track number,
// timecode, flags and lacing header, then the frames. A frame that starts exactly
// at end stops the check without an error, matching ebml-go's block reader.
func (d *ebmlDecoder) checkBlock(n int64, end int) error {
	_, nb, err := d.readEBMLVint(end)
	if err != nil {
		return err
	}
	n -= int64(nb)
	// Timecode (2 bytes) and flags (1 byte).
	if end-d.cur < 3 {
		return errEBMLTruncated
	}
	flags := d.buf[d.cur+2]
	d.cur += 3
	n -= 3
	if n < 0 {
		return errEBMLTruncated
	}

	var sizes []int64
	switch (flags & 0x06) >> 1 {
	case 0: // no lacing
		sizes = []int64{n}
	case 1: // Xiph lacing
		count, err := d.readBlockByte(end)
		if err != nil {
			return err
		}
		frames := int(count) + 1
		n--
		sizes = make([]int64, frames)
		for i := 0; i < frames-1; i++ {
			for {
				b, err := d.readBlockByte(end)
				if err != nil {
					return err
				}
				n--
				sizes[i] += int64(b)
				if b != 0xFF {
					sizes[frames-1] -= sizes[i]
					break
				}
			}
		}
		sizes[frames-1] += n
		if sizes[frames-1] <= 0 {
			return errEBMLLacing
		}
	case 2: // fixed-size lacing
		count, err := d.readBlockByte(end)
		if err != nil {
			return err
		}
		frames := int64(count) + 1
		each := (n - 1) / frames
		if each < 0 || each*frames+1 != n {
			return errEBMLLacing
		}
		sizes = make([]int64, frames)
		for i := range sizes {
			sizes[i] = each
		}
	case 3: // EBML lacing
		count, err := d.readBlockByte(end)
		if err != nil {
			return err
		}
		frames := int(count) + 1
		n--
		sizes = make([]int64, frames)
		first, nb, err := d.readEBMLVint(end)
		if err != nil {
			return err
		}
		size := int64(first)
		if size >= n {
			return errEBMLLacing
		}
		n -= int64(nb)
		sizes[0] = size
		sizes[frames-1] -= size
		for i := 1; i < frames-1; i++ {
			diff, nb, err := d.readEBMLSignedVint(end)
			size += diff
			if err != nil {
				return err
			}
			if size <= 0 || size >= n {
				return errEBMLLacing
			}
			n -= int64(nb)
			sizes[i] = size
			sizes[frames-1] -= size
		}
		sizes[frames-1] += n
		if sizes[frames-1] <= 0 {
			return errEBMLLacing
		}
	}

	for _, s := range sizes {
		if s == 0 {
			continue
		}
		if d.cur >= end {
			// The stream ended exactly at a frame boundary.
			return nil
		}
		if s > int64(end-d.cur) {
			return errEBMLTruncated
		}
		d.cur += int(s)
	}
	return nil
}

// readBlockByte reads one byte of a block lacing header.
func (d *ebmlDecoder) readBlockByte(end int) (byte, error) {
	if d.cur >= end {
		return 0, errEBMLTruncated
	}
	b := d.buf[d.cur]
	d.cur++
	return b, nil
}

// readEBMLSignedVint reads a signed EBML lacing size difference: the unsigned
// value minus the bias for its length.
func (d *ebmlDecoder) readEBMLSignedVint(end int) (int64, int, error) {
	u, nb, err := d.readEBMLVint(end)
	if err != nil {
		return 0, nb, err
	}
	bias := int64(0x007FFFFFFFFFFFFF)
	if nb < 8 {
		bias = int64(1)<<(7*nb-1) - 1
	}
	return int64(u) - bias, nb, nil
}

// uintPayload decodes a big-endian unsigned integer payload of up to 8 bytes.
func uintPayload(data []byte) uint64 {
	var v uint64
	for _, b := range data {
		v = v<<8 | uint64(b)
	}
	return v
}

// floatPayload decodes a 4- or 8-byte big-endian IEEE 754 payload.
func floatPayload(data []byte) float64 {
	if len(data) == 4 {
		return float64(math.Float32frombits(binary.BigEndian.Uint32(data)))
	}
	return math.Float64frombits(binary.BigEndian.Uint64(data))
}

// stringPayload decodes a string payload, which ends at the first NUL byte.
func stringPayload(data []byte) string {
	s, _, _ := strings.Cut(string(data), "\x00")
	return s
}
