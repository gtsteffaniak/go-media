// Package heifthumb reads embedded JPEG thumbnails from HEIF/HEIC/AVIF ISOBMFF files.
package heifthumb

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/jpegfast"
)

const (
	maxMetaBytes  = 8 << 20
	maxItemBytes  = 24 << 20
	maxTopBoxes   = 4096
	maxBoxPayload = 64 << 20
)

// JPEGPreview returns the referenced JPEG thumbnail item when present.
func JPEGPreview(r io.ReaderAt, size int64, maxW, maxH int) ([]byte, error) {
	if size <= 0 {
		return nil, errs.Corrupt
	}
	metaOff, metaEnd, err := findMetaBox(r, size)
	if err != nil {
		return nil, err
	}
	st, err := parseMeta(r, size, metaOff+4, metaEnd)
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, errs.Unsup("thumbnail", "heif: no meta thumbnail")
	}
	itemID := st.thumbItemID
	if itemID == 0 {
		itemID = st.bestJPEGItem(r, size, maxW, maxH)
	}
	if itemID == 0 {
		if st.hasCodedThumb {
			return nil, errs.Unsup("thumbnail", "heif: thumbnail item is HEVC/AV1 coded")
		}
		return nil, errs.Unsup("thumbnail", "heif: no JPEG thumbnail item")
	}
	typ := st.items[itemID]
	if isCodedVideoType(typ) {
		return nil, errs.Unsup("thumbnail", "heif: thumbnail item is HEVC/AV1 coded")
	}
	if typ != "jpeg" && typ != "jpg " && typ != "mjpg" && typ != "mime" {
		return nil, errs.Unsup("thumbnail", "heif: thumbnail item type "+typ)
	}
	data, err := st.readItem(r, size, itemID)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		return nil, errs.Unsup("thumbnail", "heif: thumbnail item is not JPEG bitstream")
	}
	return data, nil
}

type metaState struct {
	items         map[uint16]string
	thumbItemID   uint16
	hasCodedThumb bool
	iloc          ilocIndex
}

func (m *metaState) bestJPEGItem(r io.ReaderAt, size int64, maxW, maxH int) uint16 {
	var bestID uint16
	bestScore := -1
	for id, typ := range m.items {
		if typ != "jpeg" && typ != "jpg " && typ != "mjpg" {
			continue
		}
		data, err := m.readItem(r, size, id)
		if err != nil || len(data) < 4 {
			continue
		}
		cfg, err := jpegfast.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			continue
		}
		sc := fitScore(cfg.Width, cfg.Height, maxW, maxH)
		if sc > bestScore {
			bestScore = sc
			bestID = id
		}
	}
	return bestID
}

func (m *metaState) readItem(r io.ReaderAt, size int64, id uint16) ([]byte, error) {
	ext, ok := m.iloc[id]
	if !ok {
		return nil, errs.Unsup("thumbnail", "heif: iloc missing item")
	}
	if ext.length <= 0 || ext.length > maxItemBytes {
		return nil, errs.Unsup("thumbnail", "heif: item extent too large")
	}
	if ext.offset+ext.length > size {
		return nil, errs.Corrupt
	}
	b := make([]byte, ext.length)
	if _, err := r.ReadAt(b, ext.offset); err != nil {
		return nil, err
	}
	return b, nil
}

type itemExtent struct {
	offset int64
	length int64
}

type ilocIndex map[uint16]itemExtent

func findMetaBox(r io.ReaderAt, size int64) (payloadStart, payloadEnd int64, err error) {
	var off int64
	var hdr [16]byte
	for i := 0; off+8 <= size && i < maxTopBoxes; i++ {
		typ, hl, bs, err := readBoxHeader(r, size, off, size, hdr[:])
		if err != nil {
			return 0, 0, err
		}
		if typ == [4]byte{'m', 'e', 't', 'a'} {
			return off + hl, off + bs, nil
		}
		off += bs
	}
	return 0, 0, errs.Unsup("thumbnail", "heif: no meta box")
}

func parseMeta(r io.ReaderAt, size, start, end int64) (*metaState, error) {
	st := &metaState{items: map[uint16]string{}, iloc: ilocIndex{}}
	err := walkBoxes(r, size, start, end, func(typ [4]byte, ps, pe int64) error {
		switch string(typ[:]) {
		case "iinf":
			parseIinf(r, size, ps, pe, st)
		case "iref":
			parseIref(r, size, ps, pe, st)
		case "iloc":
			parseIloc(r, size, ps, pe, st)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(st.items) == 0 && len(st.iloc) == 0 {
		return nil, errs.Unsup("thumbnail", "heif: empty meta")
	}
	return st, nil
}

func parseIinf(r io.ReaderAt, size, ps, pe int64, st *metaState) {
	payload, err := readPayload(r, ps, pe, maxMetaBytes)
	if err != nil || len(payload) < 4 {
		return
	}
	ver := payload[0]
	entryCountOff := 4
	var count uint32
	if ver == 0 {
		if len(payload) < 6 {
			return
		}
		count = uint32(binary.BigEndian.Uint16(payload[4:6]))
		entryCountOff = 6
	} else {
		if len(payload) < 8 {
			return
		}
		count = binary.BigEndian.Uint32(payload[4:8])
		entryCountOff = 8
	}
	off := entryCountOff
	for i := uint32(0); i < count && off+8 <= len(payload); i++ {
		if off+8 > len(payload) {
			break
		}
		// infe is a box inside iinf payload
		boxSize := int(binary.BigEndian.Uint32(payload[off : off+4]))
		if boxSize < 8 || off+boxSize > len(payload) {
			break
		}
		boxType := string(payload[off+4 : off+8])
		if boxType == "infe" {
			parseInfe(payload[off:off+boxSize], st)
		}
		off += boxSize
	}
}

func parseInfe(box []byte, st *metaState) {
	if len(box) < 16 {
		return
	}
	ver := box[8]
	var id uint16
	var typeOff int
	switch ver {
	case 2:
		id = binary.BigEndian.Uint16(box[12:14])
		typeOff = 14
	case 1:
		id = binary.BigEndian.Uint16(box[12:14])
		typeOff = 14
	default:
		id = binary.BigEndian.Uint16(box[10:12])
		typeOff = 12
	}
	if typeOff+4 <= len(box) {
		typ := string(box[typeOff : typeOff+4])
		st.items[id] = typ
	}
}

func parseIref(r io.ReaderAt, size, ps, pe int64, st *metaState) {
	payload, err := readPayload(r, ps, pe, maxMetaBytes)
	if err != nil {
		return
	}
	off := 4 // full box
	for off+8 <= len(payload) {
		boxSize := int(binary.BigEndian.Uint32(payload[off : off+4]))
		if boxSize < 8 || off+boxSize > len(payload) {
			break
		}
		refType := string(payload[off+4 : off+8])
		body := payload[off+8 : off+boxSize]
		if refType == "thmb" && len(body) >= 6 {
			fromID := binary.BigEndian.Uint16(body[0:2])
			_ = fromID
			toCount := binary.BigEndian.Uint16(body[4:6])
			if len(body) >= 6+int(toCount)*2 {
				toID := binary.BigEndian.Uint16(body[6:8])
				st.thumbItemID = toID
				if t, ok := st.items[toID]; ok && isCodedVideoType(t) {
					st.hasCodedThumb = true
				}
			}
		}
		off += boxSize
	}
}

func parseIloc(r io.ReaderAt, size, ps, pe int64, st *metaState) {
	payload, err := readPayload(r, ps, pe, maxMetaBytes)
	if err != nil || len(payload) < 10 {
		return
	}
	ver := payload[0]
	off := 4
	var offsetSize, lengthSize, baseOffsetSize int
	if ver < 2 {
		if len(payload) < off+2 {
			return
		}
		sizes := payload[off]
		offsetSize = int(sizes >> 4)
		lengthSize = int(sizes & 0x0f)
		baseOffsetSize = 4
		if ver == 1 {
			baseOffsetSize = 8
		}
		off += 2
	} else {
		if len(payload) < off+4 {
			return
		}
		offsetSize = int(payload[off])
		lengthSize = int(payload[off+1])
		baseOffsetSize = int(payload[off+2])
		off += 4
	}
	var itemCount uint32
	if ver < 2 {
		if off+2 > len(payload) {
			return
		}
		itemCount = uint32(binary.BigEndian.Uint16(payload[off : off+2]))
		off += 2
	} else {
		if off+4 > len(payload) {
			return
		}
		itemCount = binary.BigEndian.Uint32(payload[off : off+4])
		off += 4
	}
	for i := uint32(0); i < itemCount && off < len(payload); i++ {
		if ver == 0 || ver == 1 {
			if off+2 > len(payload) {
				break
			}
			itemID := binary.BigEndian.Uint16(payload[off : off+2])
			off += 2
			if off+2 > len(payload) {
				break
			}
			off += 2 // data_reference_index
			var base int64
			if baseOffsetSize == 4 {
				if off+4 > len(payload) {
					break
				}
				base = int64(binary.BigEndian.Uint32(payload[off : off+4]))
				off += 4
			} else if baseOffsetSize == 8 {
				if off+8 > len(payload) {
					break
				}
				base = int64(binary.BigEndian.Uint64(payload[off : off+8]))
				off += 8
			}
			if off+2 > len(payload) {
				break
			}
			extCount := int(binary.BigEndian.Uint16(payload[off : off+2]))
			off += 2
			for e := 0; e < extCount && off < len(payload); e++ {
				extOff, n := readSizedInt(payload, off, offsetSize)
				off += n
				extLen, n2 := readSizedInt(payload, off, lengthSize)
				off += n2
				st.iloc[itemID] = itemExtent{offset: base + extOff, length: extLen}
			}
		}
	}
}

func readSizedInt(b []byte, off, size int) (v int64, consumed int) {
	if size == 0 {
		return 0, 0
	}
	if off+size > len(b) {
		return 0, size
	}
	var x uint64
	for i := 0; i < size; i++ {
		x = x<<8 | uint64(b[off+i])
	}
	return int64(x), size
}

func isCodedVideoType(typ string) bool {
	switch typ {
	case "hvc1", "hev1", "av01", "av1C", "hvcC", "grid":
		return true
	default:
		return false
	}
}

func fitScore(w, h, maxW, maxH int) int {
	if w <= 0 || h <= 0 {
		return 0
	}
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 {
		maxH = h
	}
	fw, fh := w, h
	if w > maxW || h > maxH {
		sx := float64(maxW) / float64(w)
		sy := float64(maxH) / float64(h)
		s := sx
		if sy < sx {
			s = sy
		}
		fw = int(float64(w)*s + 0.5)
		fh = int(float64(h)*s + 0.5)
	}
	return fw * fh
}

// readBoxHeader reads the box header at off, bounded by end.
func readBoxHeader(r io.ReaderAt, size, off, end int64, buf []byte) (typ [4]byte, hdrLen, boxSize int64, err error) {
	if end-off < 8 {
		return typ, 0, 0, errs.Corrupt
	}
	if !readFullAt(r, buf[:8], off) {
		return typ, 0, 0, errs.Corrupt
	}
	copy(typ[:], buf[4:8])
	boxSize = int64(binary.BigEndian.Uint32(buf[:4]))
	hdrLen = 8
	switch boxSize {
	case 0:
		boxSize = end - off
	case 1:
		if end-off < 16 {
			return typ, 0, 0, errs.Corrupt
		}
		if !readFullAt(r, buf[8:16], off+8) {
			return typ, 0, 0, errs.Corrupt
		}
		boxSize = int64(binary.BigEndian.Uint64(buf[8:16]))
		hdrLen = 16
	}
	if boxSize < hdrLen || boxSize > end-off {
		return typ, 0, 0, errs.Corrupt
	}
	return typ, hdrLen, boxSize, nil
}

func readFullAt(r io.ReaderAt, b []byte, off int64) bool {
	n, err := r.ReadAt(b, off)
	return n == len(b) && (err == nil || err == io.EOF)
}

func walkBoxes(r io.ReaderAt, size, start, end int64, fn func(typ [4]byte, payload, payloadEnd int64) error) error {
	var hdr [16]byte
	for off := start; off+8 <= end; {
		typ, hdrLen, boxSize, err := readBoxHeader(r, size, off, end, hdr[:])
		if err != nil {
			return err
		}
		if err := fn(typ, off+hdrLen, off+boxSize); err != nil {
			return err
		}
		off += boxSize
	}
	return nil
}

func readPayload(r io.ReaderAt, start, end int64, limit int64) ([]byte, error) {
	n := end - start
	if n < 0 || n > limit || n > maxBoxPayload {
		return nil, errs.Corrupt
	}
	b := make([]byte, n)
	if n > 0 && !readFullAt(r, b, start) {
		return nil, errs.Corrupt
	}
	return b, nil
}
