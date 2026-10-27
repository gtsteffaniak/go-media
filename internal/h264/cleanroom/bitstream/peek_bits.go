package bitstream

import "encoding/binary"

// peekBitsMSB reads up to 32 bits MSB-first from data at bit position pos.
func peekBitsMSB(data []byte, pos int, n int) uint32 {
	if n <= 0 {
		return 0
	}
	if n > 32 {
		n = 32
	}
	total := len(data) * 8
	bi := pos >> 3
	bo := pos & 7
	// Fast path: full 8-byte window available and all n bits within stream.
	if pos+n <= total && bi+8 <= len(data) {
		x := binary.BigEndian.Uint64(data[bi : bi+8])
		if bo != 0 {
			x <<= uint(bo)
		}
		return uint32(x >> (64 - n))
	}
	return peekBitsMSBSlow(data, pos, n)
}

func peekBitsMSBSlow(data []byte, pos int, n int) uint32 {
	total := len(data) * 8
	var v uint32
	for i := 0; i < n; i++ {
		p := pos + i
		var bit uint32
		if p < total {
			bit = uint32((data[p>>3] >> uint(7-(p&7))) & 1)
		}
		v = (v << 1) | bit
	}
	return v
}
