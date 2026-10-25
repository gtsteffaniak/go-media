// Package bitstream implements a zero-allocation RBSP bit reader with a 64-bit cache.
package bitstream

import "fmt"

// Reader reads H.264 RBSP (emulation prevention already removed).
// Valid bits are MSB-aligned in the 64-bit register (ffmpeg-style).
type Reader struct {
	data []byte
	pos  int
	reg  uint64
	nb   int // number of valid bits in reg (from the MSB side)
}

// New attaches a reader to rbsp without copying.
func New(rbsp []byte) *Reader {
	r := &Reader{data: rbsp}
	r.load()
	return r
}

func (r *Reader) load() {
	for r.nb <= 56 && r.pos < len(r.data) {
		r.reg |= uint64(r.data[r.pos]) << (56 - r.nb)
		r.pos++
		r.nb += 8
	}
}

func (r *Reader) ensure(n int) error {
	for r.nb < n {
		if r.pos >= len(r.data) {
			return fmt.Errorf("bitstream: overrun")
		}
		r.load()
	}
	return nil
}

// Read reads n bits (1..32).
func (r *Reader) Read(n int) (uint32, error) {
	if n <= 0 || n > 32 {
		return 0, fmt.Errorf("bitstream: invalid read width %d", n)
	}
	if err := r.ensure(n); err != nil {
		return 0, err
	}
	v := uint32(r.reg >> (64 - n))
	r.reg <<= uint(n)
	r.nb -= n
	r.load()
	return v, nil
}

// Read1 reads a single bit.
func (r *Reader) Read1() (uint32, error) {
	return r.Read(1)
}

// Skip discards n bits.
func (r *Reader) Skip(n int) error {
	if n <= 0 {
		return nil
	}
	if err := r.ensure(n); err != nil {
		return err
	}
	r.reg <<= uint(n)
	r.nb -= n
	r.load()
	return nil
}

// ReadUE reads unsigned Exp-Golomb code.
func (r *Reader) ReadUE() (uint32, error) {
	zeros := 0
	for {
		b, err := r.Read1()
		if err != nil {
			return 0, err
		}
		if b == 1 {
			break
		}
		zeros++
		if zeros > 31 {
			return 0, fmt.Errorf("bitstream: ue overflow")
		}
	}
	if zeros == 0 {
		return 0, nil
	}
	val, err := r.Read(zeros)
	if err != nil {
		return 0, err
	}
	return (1 << zeros) - 1 + val, nil
}

// ReadSE reads signed Exp-Golomb code.
func (r *Reader) ReadSE() (int32, error) {
	ue, err := r.ReadUE()
	if err != nil {
		return 0, err
	}
	if ue&1 == 0 {
		return -int32(ue >> 1), nil
	}
	return int32(ue+1 >> 1), nil
}

// PeekBits returns the next n bits without consuming them (n <= 32).
func (r *Reader) PeekBits(n int) (uint32, error) {
	if n <= 0 || n > 32 {
		return 0, fmt.Errorf("bitstream: peek width %d", n)
	}
	if err := r.ensure(n); err != nil {
		return 0, err
	}
	return uint32(r.reg >> (64 - n)), nil
}

// SkipBits discards n bits (fast path for VLC).
func (r *Reader) SkipBits(n int) error {
	return r.Skip(n)
}

// MoreRBSPData implements H.264 more_rbsp_data() using the 64-bit cache.
func (r *Reader) MoreRBSPData() bool {
	if r.nb > 0 && r.reg != 0 {
		return true
	}
	return r.pos < len(r.data)
}

// ByteAlign skips to the next byte boundary (cabac alignment).
func (r *Reader) ByteAlign() error {
	skip := r.nb & 7
	if skip != 0 {
		return r.Skip(skip)
	}
	return nil
}
