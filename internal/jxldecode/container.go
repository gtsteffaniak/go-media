package jxldecode

import (
	"encoding/binary"
	"errors"
	"io"
)

// maxBoxes bounds how many ISO BMFF boxes codestreamSpan walks before the
// codestream. Real files have a handful (signature, ftyp, metadata, jxlc).
const maxBoxes = 256

// codestreamSpan locates the codestream inside r and returns its offset and
// length. A bare codestream starts at 0. For a container it walks the box
// headers only (8 or 16 bytes each, read with ReadAt), so a large leading box
// such as ICC, Exif or XMP is skipped without reading its payload. ok is false
// when the layout cannot be resolved within maxBoxes boxes or the box sizes
// are inconsistent with size.
func codestreamSpan(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (off, length int64, ok bool) {
	var sig [2]byte
	if n, err := r.ReadAt(sig[:], 0); n < 2 && err != nil {
		return 0, 0, false
	}
	if sig[0] == 0xFF && sig[1] == 0x0A {
		return 0, size, true
	}

	pos := int64(0)
	for i := 0; i < maxBoxes && pos+8 <= size; i++ {
		var h [16]byte
		n := min(int64(len(h)), size-pos)
		if _, err := r.ReadAt(h[:n], pos); err != nil && !errors.Is(err, io.EOF) {
			return 0, 0, false
		}
		boxSize := uint64(binary.BigEndian.Uint32(h[0:4]))
		typ := string(h[4:8])
		hdr := int64(8)
		if boxSize == 1 {
			if n < 16 {
				return 0, 0, false
			}
			boxSize = binary.BigEndian.Uint64(h[8:16])
			hdr = 16
		}
		end := size // size 0 means the box runs to end of file
		if boxSize != 0 {
			// Compare in uint64 before converting: a largesize near 2^64 must
			// not wrap to a small or negative offset.
			if boxSize < uint64(hdr) || boxSize > uint64(size-pos) {
				return 0, 0, false
			}
			end = pos + int64(boxSize)
		}

		var start int64
		switch typ {
		case "jxlc":
			start = pos + hdr
		case "jxlp":
			// A jxlp payload begins with a 4-byte index before the codestream.
			start = pos + hdr + 4
		default:
			pos = end
			continue
		}
		if start > end {
			return 0, 0, false
		}
		return start, end - start, true
	}
	return 0, 0, false
}
