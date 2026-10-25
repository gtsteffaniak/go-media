// Package bitstream implements a zero-allocation RBSP bit reader with a 64-bit cache.
package bitstream

import "fmt"

// Reader reads H.264 RBSP (emulation prevention already removed).
type Reader struct {
	data []byte
	pos  int // byte offset into data
	bits uint64
	nb   int // valid bits in bits (MSB-aligned)
}

// New attaches a reader to rbsp without copying.
func New(rbsp []byte) *Reader {
	r := &Reader{data: rbsp}
	r.refill()
	return r
}

func (r *Reader) refill() {
	for r.nb <= 32 && r.pos < len(r.data) {
		r.bits = (r.bits << 8) | uint64(r.data[r.pos])
		r.pos++
		r.nb += 8
	}
}

// Available returns true if at least n bits can be read.
func (r *Reader) Available(n int) bool {
	for r.nb < n && r.pos < len(r.data) {
		r.refill()
	}
	return r.nb >= n
}

// Read reads n bits (1..32).
func (r *Reader) Read(n int) (uint32, error) {
	if n <= 0 || n > 32 {
		return 0, fmt.Errorf("bitstream: invalid read width %d", n)
	}
	for r.nb < n {
		if r.pos >= len(r.data) {
			return 0, fmt.Errorf("bitstream: overrun")
		}
		r.refill()
	}
	r.nb -= n
	return uint32(r.bits >> r.nb), nil
}

// Read1 reads a single bit.
func (r *Reader) Read1() (uint32, error) {
	return r.Read(1)
}

// Skip discards n bits.
func (r *Reader) Skip(n int) error {
	for n > 0 {
		if r.nb == 0 {
			if r.pos >= len(r.data) {
				return fmt.Errorf("bitstream: skip overrun")
			}
			r.refill()
		}
		take := n
		if take > r.nb {
			take = r.nb
		}
		r.nb -= take
		n -= take
	}
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

// MoreRBSPData implements H.264 more_rbsp_data() using the 64-bit cache.
func (r *Reader) MoreRBSPData() bool {
	if r.nb > 0 {
		// Any non-padding bit before byte alignment means more data.
		if r.bits<<(64-r.nb) != 0 {
			return true
		}
	}
	if r.pos < len(r.data) {
		return true
	}
	return false
}

// ByteAlign skips to the next byte boundary (cabac alignment).
func (r *Reader) ByteAlign() error {
	skip := r.nb & 7
	if skip != 0 {
		return r.Skip(skip)
	}
	return nil
}
