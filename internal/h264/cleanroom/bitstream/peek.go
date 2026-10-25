package bitstream

import "fmt"

// PeekBits returns the next n bits without consuming them (n <= 32).
func (r *Reader) PeekBits(n int) (uint32, error) {
	if n <= 0 || n > 32 {
		return 0, fmt.Errorf("bitstream: peek width %d", n)
	}
	for r.nb < n {
		if r.pos >= len(r.data) {
			return 0, fmt.Errorf("bitstream: peek overrun")
		}
		r.refill()
	}
	return uint32(r.bits >> (r.nb - n)), nil
}

// SkipBits discards n bits (fast path for VLC).
func (r *Reader) SkipBits(n int) error {
	return r.Skip(n)
}
