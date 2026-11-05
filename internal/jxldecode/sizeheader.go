package jxldecode

import "encoding/binary"

// headerSize reads the JPEG XL SizeHeader (ISO/IEC 18181-1 §A.2) from a bare
// codestream or from the first jxlc/jxlp box of a container in prefix. It is
// used only to name dimensions in errors for headers the decoder refuses.
func headerSize(p []byte) (w, h int, ok bool) {
	cs, ok := codestreamStart(p)
	if !ok || len(cs) < 2 || cs[0] != 0xFF || cs[1] != 0x0A {
		return 0, 0, false
	}
	br := bitReader{b: cs, pos: 16}
	u32 := func() uint64 {
		n := [4]uint{9, 13, 18, 30}[br.bits(2)]
		return br.bits(n) + 1
	}
	small := br.bits(1) == 1
	var y uint64
	if small {
		y = (br.bits(5) + 1) * 8
	} else {
		y = u32()
	}
	var x uint64
	switch ratio := br.bits(3); ratio {
	case 0:
		if small {
			x = (br.bits(5) + 1) * 8
		} else {
			x = u32()
		}
	case 1:
		x = y
	case 2:
		x = y * 12 / 10
	case 3:
		x = y * 4 / 3
	case 4:
		x = y * 3 / 2
	case 5:
		x = y * 16 / 9
	case 6:
		x = y * 5 / 4
	case 7:
		x = y * 2
	}
	if br.short || x == 0 || x > 1<<32-1 {
		return 0, 0, false
	}
	return int(x), int(y), true
}

// codestreamStart returns the codestream bytes for a bare stream, or the
// payload of the first jxlc/jxlp box of an ISO BMFF container.
func codestreamStart(p []byte) ([]byte, bool) {
	if len(p) >= 2 && p[0] == 0xFF && p[1] == 0x0A {
		return p, true
	}
	for off := 0; off+8 <= len(p); {
		sz := uint64(binary.BigEndian.Uint32(p[off:]))
		typ := string(p[off+4 : off+8])
		hdr := uint64(8)
		if sz == 1 {
			if off+16 > len(p) {
				return nil, false
			}
			sz, hdr = binary.BigEndian.Uint64(p[off+8:]), 16
		}
		body := off + int(hdr)
		switch typ {
		case "jxlc":
			return p[body:], true
		case "jxlp":
			if body+4 > len(p) {
				return nil, false
			}
			return p[body+4:], true
		}
		if sz < hdr || sz == 0 {
			return nil, false
		}
		off += int(sz)
	}
	return nil, false
}

type bitReader struct {
	b     []byte
	pos   uint
	short bool
}

func (r *bitReader) bits(n uint) uint64 {
	var v uint64
	for i := uint(0); i < n; i++ {
		if int(r.pos/8) >= len(r.b) {
			r.short = true
			return 0
		}
		v |= uint64(r.b[r.pos/8]>>(r.pos%8)&1) << i
		r.pos++
	}
	return v
}
