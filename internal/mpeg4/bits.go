package mpeg4

import "encoding/binary"

// reader reads MSB-first bits from a byte slice. Reads past the end return
// zero bits and set over; callers check over once per macroblock so a
// truncated packet cannot loop or index out of range.
type reader struct {
	buf  []byte
	pos  int // bit position
	over bool
}

func newReader(b []byte) reader { return reader{buf: b} }

// peek returns the next n (1..32) bits without consuming them, zero-padded
// past the end of the buffer.
func (r *reader) peek(n int) uint32 {
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
	return uint32((v << uint(r.pos&7)) >> uint(64-n))
}

// skip consumes n bits.
func (r *reader) skip(n int) {
	r.pos += n
	if r.pos > len(r.buf)*8 {
		r.over = true
	}
}

// bits reads n (0..32) bits.
func (r *reader) bits(n int) uint32 {
	if n == 0 {
		return 0
	}
	v := r.peek(n)
	r.skip(n)
	return v
}

// bit reads one bit.
func (r *reader) bit() bool { return r.bits(1) != 0 }

// left returns the number of unread bits (negative after an overread).
func (r *reader) left() int { return len(r.buf)*8 - r.pos }

// marker reads a marker bit and reports whether it was 1.
func (r *reader) marker() bool { return r.bit() }
