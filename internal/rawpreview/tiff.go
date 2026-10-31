package rawpreview

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/jpegfast"
)

const (
	tagImageWidth               = 0x0100
	tagImageLength              = 0x0101
	tagCompression              = 0x0103
	tagJPEGInterchangeFormat    = 0x0201
	tagJPEGInterchangeFormatLen = 0x0202
	tagSubIFDs                  = 0x014a
	compJPEG                    = 6
	compOldJPEG                 = 7
	maxTIFFRead                 = 64 << 20
	maxIFDs                     = 64
)

func tiffJPEGCandidates(r io.ReaderAt, size int64) ([]jpegCand, error) {
	if size < 8 {
		return nil, errs.Corrupt
	}
	head := make([]byte, 8)
	if _, err := r.ReadAt(head, 0); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch {
	case head[0] == 'I' && head[1] == 'I':
		order = binary.LittleEndian
	case head[0] == 'M' && head[1] == 'M':
		order = binary.BigEndian
	default:
		return nil, nil
	}
	if order.Uint16(head[2:4]) != 42 {
		return nil, nil
	}
	ifd0 := int64(order.Uint32(head[4:8]))
	return walkIFDs(r, size, order, ifd0, 0)
}

func walkIFDs(r io.ReaderAt, size int64, order binary.ByteOrder, ifdOff int64, depth int) ([]jpegCand, error) {
	if depth > 4 || ifdOff <= 0 || ifdOff+2 > size {
		return nil, nil
	}
	var out []jpegCand
	for n := 0; n < maxIFDs && ifdOff > 0 && ifdOff+2 <= size; n++ {
		countBuf := make([]byte, 2)
		if _, err := r.ReadAt(countBuf, ifdOff); err != nil {
			return out, err
		}
		count := int(order.Uint16(countBuf))
		entryOff := ifdOff + 2
		if entryOff+int64(count)*12 > size {
			break
		}
		var jpegOff, jpegLen int64
		var subIFDs []int64
		for i := 0; i < count; i++ {
			ent := make([]byte, 12)
			off := entryOff + int64(i)*12
			if _, err := r.ReadAt(ent, off); err != nil {
				return out, err
			}
			tag := order.Uint16(ent[0:2])
			typ := order.Uint16(ent[2:4])
			cnt := order.Uint32(ent[4:8])
			valOff := ent[8:12]
			cnt16 := uint16(cnt)
			switch tag {
			case tagJPEGInterchangeFormat:
				jpegOff = int64(readIFDValue(order, typ, cnt16, valOff))
			case tagJPEGInterchangeFormatLen:
				jpegLen = int64(readIFDValue(order, typ, cnt16, valOff))
			case tagSubIFDs:
				subIFDs = append(subIFDs, readIFDOffsets(order, typ, cnt16, valOff, r, size)...)
			case tagCompression:
				if readIFDValue(order, typ, cnt16, valOff) == compJPEG || readIFDValue(order, typ, cnt16, valOff) == compOldJPEG {
					// strip/tile JPEG may follow StripOffsets; handled below via jpeg tags.
				}
			}
		}
		if jpegOff > 0 && jpegLen > 0 && jpegOff+jpegLen <= size && jpegLen <= maxJPEGBytes {
			data, err := readAt(r, jpegOff, jpegLen)
			if err == nil {
				if c, ok := candidateFromBytes(data); ok {
					out = append(out, c)
				}
			}
		}
		for _, sub := range subIFDs {
			subC, _ := walkIFDs(r, size, order, sub, depth+1)
			out = append(out, subC...)
		}
		nextOff := entryOff + int64(count)*12
		if nextOff+4 > size {
			break
		}
		next := make([]byte, 4)
		if _, err := r.ReadAt(next, nextOff); err != nil {
			break
		}
		ifdOff = int64(order.Uint32(next))
	}
	return out, nil
}

func readIFDValue(order binary.ByteOrder, typ, cnt uint16, valOff []byte) uint32 {
	if typ == 3 && cnt == 1 { // SHORT inline
		if order == binary.LittleEndian {
			return uint32(valOff[0]) | uint32(valOff[1])<<8
		}
		return uint32(valOff[1]) | uint32(valOff[0])<<8
	}
	if typ == 4 && cnt == 1 { // LONG inline
		return order.Uint32(valOff)
	}
	return order.Uint32(valOff)
}

func readIFDOffsets(order binary.ByteOrder, typ, cnt uint16, valOff []byte, r io.ReaderAt, size int64) []int64 {
	var out []int64
	switch {
	case typ == 4 && cnt == 1:
		out = append(out, int64(order.Uint32(valOff)))
	case typ == 4 && cnt > 1:
		base := int64(order.Uint32(valOff))
		buf := make([]byte, int(cnt)*4)
		if base+int64(len(buf)) > size {
			return out
		}
		if _, err := r.ReadAt(buf, base); err != nil {
			return out
		}
		for i := uint16(0); i < cnt; i++ {
			out = append(out, int64(order.Uint32(buf[int(i)*4:int(i)*4+4])))
		}
	case typ == 13 && cnt >= 1: // IFD offset type in DNG
		base := int64(order.Uint32(valOff))
		if cnt == 1 {
			out = append(out, base)
		}
	}
	return out
}

func scanJPEGCandidates(r io.ReaderAt, size int64) ([]jpegCand, error) {
	limit := size
	if limit > maxScanBytes {
		limit = maxScanBytes
	}
	buf := make([]byte, limit)
	n, _ := ioat.ReadRange(r, size, 0, buf)
	buf = buf[:n]
	var out []jpegCand
	for i := 0; i+2 < len(buf); i++ {
		if buf[i] != 0xff || buf[i+1] != 0xd8 {
			continue
		}
		end := findJPEGEnd(buf, i)
		if end <= i+2 {
			continue
		}
		chunk := buf[i:end]
		if len(chunk) > maxJPEGBytes {
			continue
		}
		if c, ok := candidateFromBytes(chunk); ok {
			out = append(out, c)
		}
	}
	return out, nil
}

func findJPEGEnd(b []byte, start int) int {
	for i := start + 2; i+1 < len(b); i++ {
		if b[i] == 0xff && b[i+1] == 0xd9 {
			return i + 2
		}
	}
	return len(b)
}

func jpegDimensions(data []byte) (w, h int, err error) {
	cfg, err := jpegfast.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
