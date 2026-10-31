package vc1

import "encoding/binary"

type bitReader struct {
	buf  []byte
	pos  int
	over bool
}

func newBitReader(b []byte) bitReader { return bitReader{buf: b} }

func (r *bitReader) peek(n int) uint32 {
	i := r.pos >> 3
	var v uint64
	if i+8 <= len(r.buf) {
		v = binary.BigEndian.Uint64(r.buf[i:])
	} else {
		for k := 0; k < 8; k++ {
			v <<= 8
			if i+k < len(r.buf) {
				v |= uint64(r.buf[i+k])
			}
		}
	}
	w := (v << uint(r.pos&7)) >> uint(64-n)
	if n < 32 {
		w &= (1 << uint(n)) - 1
	}
	return uint32(w)
}

func (r *bitReader) skip(n int) {
	r.pos += n
	if r.pos > len(r.buf)*8 {
		r.over = true
	}
}

func (r *bitReader) bits(n int) uint32 {
	if n == 0 {
		return 0
	}
	v := r.peek(n)
	r.skip(n)
	return v
}

func (r *bitReader) bit() bool { return r.bits(1) != 0 }

func (r *bitReader) left() int { return len(r.buf)*8 - r.pos }
