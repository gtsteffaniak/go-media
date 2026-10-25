// Package bitstream — reads H.264 fields from RBSP (bytes already without
// emulation-prevention). Bits go MSB-first within a byte — that is the order
// defined by Annex-B / ISO/IEC 14496-10. Pure Go, no dependencies.
//
// Reader uses a "sticky" error: after the first read past the end of data,
// all subsequent reads return 0 and do not advance the position, while Err()
// reports the cause. This lets syntax parsing be written linearly, without
// checking the error after each field — a single Err() at the end is enough.
package bitstream

import "errors"

// ErrEOF — attempt to read past the end of data.
var ErrEOF = errors.New("bitstream: read past the end of data")

var (
	errBitsRange = errors.New("bitstream: U(n) — n out of range 0..32")
	errExpGolomb = errors.New("bitstream: invalid Exp-Golomb (corrupt bitstream)")
)

// Reader reads bits from RBSP sequentially.
type Reader struct {
	data []byte
	pos  int // position in bits from the start of data
	err  error
}

// NewReader creates a reader over RBSP.
func NewReader(data []byte) *Reader { return &Reader{data: data} }

// Err returns the first error that occurred (sticky), or nil.
func (r *Reader) Err() error { return r.err }

// BitPos — the current position in bits.
func (r *Reader) BitPos() int { return r.pos }

// BitsLeft — how many bits remain until the end of data.
func (r *Reader) BitsLeft() int { return len(r.data)*8 - r.pos }

// readBit reads one bit; on reading past the end of data it sets ErrEOF and
// returns 0 (the position does not advance).
func (r *Reader) readBit() uint32 {
	if r.err != nil {
		return 0
	}
	if r.pos >= len(r.data)*8 {
		r.err = ErrEOF
		return 0
	}
	b := r.data[r.pos>>3]
	shift := uint(7 - (r.pos & 7))
	r.pos++
	return uint32((b >> shift) & 1)
}

// Bit reads one bit as 0/1.
func (r *Reader) Bit() uint32 { return r.readBit() }

// U reads n bits (0..32) as an unsigned integer, MSB-first — this is u(n).
func (r *Reader) U(n int) uint32 {
	if r.err != nil {
		return 0
	}
	if n < 0 || n > 32 {
		r.err = errBitsRange
		return 0
	}
	var v uint32
	for i := 0; i < n; i++ {
		v = (v << 1) | r.readBit()
	}
	return v
}

// Flag reads one bit as bool — this is u(1).
func (r *Reader) Flag() bool { return r.readBit() == 1 }

// UE reads an unsigned Exp-Golomb — this is ue(v).
// Encoding: M leading zeros, then 1, then M bits of INFO;
// value = 2^M − 1 + INFO.
func (r *Reader) UE() uint32 {
	if r.err != nil {
		return 0
	}
	zeros := 0
	for r.readBit() == 0 {
		if r.err != nil {
			return 0
		}
		zeros++
		if zeros > 31 { // 32+ leading zeros do not fit in uint32 — corrupt bitstream
			r.err = errExpGolomb
			return 0
		}
	}
	if zeros == 0 {
		return 0
	}
	suffix := r.U(zeros)
	return (uint32(1) << uint(zeros)) - 1 + suffix
}

// SE reads a signed Exp-Golomb — this is se(v).
// Mapping codeNum→value: 0→0, 1→+1, 2→−1, 3→+2, 4→−2 …
func (r *Reader) SE() int32 {
	k := r.UE()
	if k&1 == 1 {
		return int32((k + 1) / 2)
	}
	return -int32(k / 2)
}

// PeekBits returns the next n bits as unsigned, WITHOUT advancing the position.
// Bits missing at the end of the bitstream are treated as zeros (needed for
// table-based variable-length-code parsing, where a fixed-length window is peeked first).
func (r *Reader) PeekBits(n int) uint32 {
	var v uint32
	total := len(r.data) * 8
	for i := 0; i < n; i++ {
		var bit uint32
		if p := r.pos + i; p < total {
			bit = uint32((r.data[p>>3] >> uint(7-(p&7))) & 1)
		}
		v = (v << 1) | bit
	}
	return v
}

// Skip advances the position by n bits (flush after PeekBits).
func (r *Reader) Skip(n int) {
	r.pos += n
	if total := len(r.data) * 8; r.pos > total {
		r.pos = total
		r.err = ErrEOF
	}
}

// ByteAligned reports that the position sits on a byte boundary.
func (r *Reader) ByteAligned() bool { return r.pos%8 == 0 }

// AlignToByte advances to the nearest byte boundary (for pcm_alignment_zero_bit
// and cabac_alignment_one_bit the skipped bits' values do not matter to us).
func (r *Reader) AlignToByte() {
	r.pos = (r.pos + 7) &^ 7
}

// MoreRBSPData reports whether syntax data remains in the bitstream before
// rbsp_trailing_bits. Logic: rbsp_trailing_bits is one "1" bit
// (rbsp_stop_one_bit), then zeros. Data is present if the current position is
// before the last set bit.
func (r *Reader) MoreRBSPData() bool {
	if r.err != nil {
		return false
	}
	total := len(r.data) * 8
	if r.pos >= total {
		return false
	}
	last := -1
	for i := total - 1; i >= r.pos; i-- {
		if (r.data[i>>3]>>uint(7-(i&7)))&1 == 1 {
			last = i
			break
		}
	}
	if last < 0 {
		return false // only zeros remain — this is already the trailing part
	}
	return r.pos < last
}
