package decode

import (
	"encoding/binary"
	"math/bits"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

// CABAC arithmetic decoding engine (ISO/IEC 14496-10). The spec's 9-bit codIOffset and
// codIRange in [256,510] are kept, but the offset is held scaled: value carries
// codIOffset in its high bits followed by `bits` prefetched stream bits, so
// codIOffset == value>>bits. Renormalization is then a single shift of `bits` (the
// spec's bit-by-bit loop) with the shift count taken from a leading-zero count, and
// stream bytes are loaded 32 bits at a time instead of one bit at a time. The
// state-transition and LPS-range tables are ported verbatim from OpenH264
// (common_tables.cpp: g_kuiCabacRangeLps, g_kuiStateTransTable).

// rangeTabLPS[pStateIdx][(codIRange>>6)&3] — LPS sub-range.
var rangeTabLPS = [64][4]uint8{
	{128, 176, 208, 240}, {128, 167, 197, 227}, {128, 158, 187, 216}, {123, 150, 178, 205},
	{116, 142, 169, 195}, {111, 135, 160, 185}, {105, 128, 152, 175}, {100, 122, 144, 166},
	{95, 116, 137, 158}, {90, 110, 130, 150}, {85, 104, 123, 142}, {81, 99, 117, 135},
	{77, 94, 111, 128}, {73, 89, 105, 122}, {69, 85, 100, 116}, {66, 80, 95, 110},
	{62, 76, 90, 104}, {59, 72, 86, 99}, {56, 69, 81, 94}, {53, 65, 77, 89},
	{51, 62, 73, 85}, {48, 59, 69, 80}, {46, 56, 66, 76}, {43, 53, 63, 72},
	{41, 50, 59, 69}, {39, 48, 56, 65}, {37, 45, 54, 62}, {35, 43, 51, 59},
	{33, 41, 48, 56}, {32, 39, 46, 53}, {30, 37, 43, 50}, {29, 35, 41, 48},
	{27, 33, 39, 45}, {26, 31, 37, 43}, {24, 30, 35, 41}, {23, 28, 33, 39},
	{22, 27, 32, 37}, {21, 26, 30, 35}, {20, 24, 29, 33}, {19, 23, 27, 31},
	{18, 22, 26, 30}, {17, 21, 25, 28}, {16, 20, 23, 27}, {15, 19, 22, 25},
	{14, 18, 21, 24}, {14, 17, 20, 23}, {13, 16, 19, 22}, {12, 15, 18, 21},
	{12, 14, 17, 20}, {11, 14, 16, 19}, {11, 13, 15, 18}, {10, 12, 15, 17},
	{10, 12, 14, 16}, {9, 11, 13, 15}, {9, 11, 12, 14}, {8, 10, 12, 14},
	{8, 9, 11, 13}, {7, 9, 11, 12}, {7, 9, 10, 12}, {7, 8, 10, 11},
	{6, 8, 9, 11}, {6, 7, 9, 10}, {6, 7, 8, 9}, {2, 2, 2, 2},
}

// stateTrans[state] = {transIdxLPS, transIdxMPS} — next pStateIdx after LPS/MPS.
var stateTrans = [64][2]uint8{
	{0, 1}, {0, 2}, {1, 3}, {2, 4}, {2, 5}, {4, 6}, {4, 7}, {5, 8}, {6, 9}, {7, 10},
	{8, 11}, {9, 12}, {9, 13}, {11, 14}, {11, 15}, {12, 16}, {13, 17}, {13, 18}, {15, 19}, {15, 20},
	{16, 21}, {16, 22}, {18, 23}, {18, 24}, {19, 25}, {19, 26}, {21, 27}, {21, 28}, {22, 29}, {22, 30},
	{23, 31}, {24, 32}, {24, 33}, {25, 34}, {26, 35}, {26, 36}, {27, 37}, {27, 38}, {28, 39}, {29, 40},
	{29, 41}, {30, 42}, {30, 43}, {30, 44}, {31, 45}, {32, 46}, {32, 47}, {33, 48}, {33, 49}, {33, 50},
	{34, 51}, {34, 52}, {35, 53}, {35, 54}, {35, 55}, {36, 56}, {36, 57}, {36, 58}, {37, 59}, {37, 60},
	{37, 61}, {38, 62}, {38, 62}, {63, 63},
}

// cabacCtx — one context model: probability state index and the MPS value.
type cabacCtx struct {
	state uint8 // pStateIdx 0..63
	mps   uint8 // valMPS 0/1
}

const numCabacCtx = 460

// cabacEngine — the binary arithmetic decoder and the context model array.
//
// Invariant between calls: value < codIRange<<bits and bits >= 0. The stream is
// zero-padded past the end of data; err reports whether decoding consumed more bits
// than the data held (the same condition that set the bit reader's sticky ErrEOF).
type cabacEngine struct {
	data      []byte
	p         int // next byte of data to load; may run past len(data) (zero padding)
	value     uint64
	bits      int
	codIRange uint32
	ctx       [numCabacCtx]cabacCtx
}

// resetCabacEngine initializes registers and context models for a new slice.
// The RBSP must already be byte aligned (cabac_alignment_one_bit consumed). The engine
// takes over the reader's remaining bytes; r's position is not advanced.
func (e *cabacEngine) resetCabacEngine(r *bitstream.Reader, sliceQP, model int) {
	e.data = r.AlignedBytes()
	e.p = 0
	e.value = 0
	e.bits = -9 // the initial 9-bit codIOffset is read below
	e.codIRange = 510
	e.refill()
	e.initContexts(sliceQP, model)
}

// newCabacDecoder builds an engine with the full 460 context models and
// initializes them from sliceQP (ISO/IEC 14496-10). model selects the init set:
// 0 for an I-slice, cabac_init_idc+1 for P/B.
func newCabacDecoder(r *bitstream.Reader, sliceQP, model int) *cabacEngine {
	e := &cabacEngine{}
	e.resetCabacEngine(r, sliceQP, model)
	return e
}

// refill tops up the prefetch window. Callers invoke it when bits < 8, the most any
// single operation consumes, so value never holds more than 9+bits+32 <= 64 bits.
func (e *cabacEngine) refill() {
	if e.bits >= 0 && e.p+4 <= len(e.data) {
		e.value = e.value<<32 | uint64(binary.BigEndian.Uint32(e.data[e.p:]))
		e.p += 4
		e.bits += 32
		return
	}
	for e.bits < 24 {
		var b byte
		if e.p < len(e.data) {
			b = e.data[e.p]
		}
		e.p++
		e.value = e.value<<8 | uint64(b)
		e.bits += 8
	}
}

// err returns bitstream.ErrEOF if decoding has consumed bits beyond the end of data.
func (e *cabacEngine) err() error {
	// Spec-consumed bits = loaded bits minus the prefetched ones still in the window.
	if e.p*8-e.bits > len(e.data)*8 {
		return bitstream.ErrEOF
	}
	return nil
}

// offset returns the spec's 9-bit codIOffset.
func (e *cabacEngine) offset() uint32 { return uint32(e.value >> uint(e.bits)) }

// initContexts derives each context's (pStateIdx, valMPS) from the (m,n)
// constants and the slice QP.
func (e *cabacEngine) initContexts(sliceQP, model int) {
	qp := clip3(0, 51, sliceQP)
	for i := range e.ctx {
		m := int(cabacInitConsts[i][model][0])
		n := int(cabacInitConsts[i][model][1])
		pre := clip3(1, 126, ((m*qp)>>4)+n)
		if pre <= 63 {
			e.ctx[i] = cabacCtx{state: uint8(63 - pre), mps: 0}
		} else {
			e.ctx[i] = cabacCtx{state: uint8(pre - 64), mps: 1}
		}
	}
}

// decodeDecision decodes one regular (context-coded) bin for context ctxIdx.
//
// The LPS/MPS split is a plain branch on purpose: a branch-free mask-arithmetic variant
// (as ffmpeg's x86 cmov path does) was bit-exact but ~8% slower end to end on arm64,
// where the branch predictor copes and the longer dependency chain costs more. It has
// not been measured on amd64; re-test there before changing this.
func (e *cabacEngine) decodeDecision(ctxIdx int) uint32 {
	if e.bits < 8 {
		e.refill()
	}
	c := &e.ctx[ctxIdx]
	st := c.state
	rng := e.codIRange
	rangeLPS := uint32(rangeTabLPS[st][(rng>>6)&3])
	rng -= rangeLPS
	scaled := uint64(rng) << uint(e.bits)

	var binVal uint32
	if e.value >= scaled {
		binVal = uint32(1 - c.mps)
		e.value -= scaled
		rng = rangeLPS
		if st == 0 {
			c.mps = 1 - c.mps
		}
		c.state = stateTrans[st][0]
	} else {
		binVal = uint32(c.mps)
		c.state = stateTrans[st][1]
	}
	// Renormalize: the spec shifts range left until it is >= 256, one stream bit per
	// shift. With the scaled offset that is the same shift applied to range and a
	// matching decrease of bits (rng is in [2,510], so n is 0..7).
	n := bits.LeadingZeros32(rng) - 23
	e.codIRange = rng << uint(n)
	e.bits -= n
	return binVal
}

// decodeBypass decodes one bypass (equiprobable) bin — no context, no renorm
// of codIRange.
func (e *cabacEngine) decodeBypass() uint32 {
	if e.bits < 8 {
		e.refill()
	}
	e.bits--
	scaled := uint64(e.codIRange) << uint(e.bits)
	if e.value >= scaled {
		e.value -= scaled
		return 1
	}
	return 0
}

// decodeTerminate decodes the end_of_slice_flag / I_PCM terminator. Returns 1
// when decoding terminates.
func (e *cabacEngine) decodeTerminate() uint32 {
	if e.bits < 8 {
		e.refill()
	}
	e.codIRange -= 2
	if e.value >= uint64(e.codIRange)<<uint(e.bits) {
		return 1
	}
	n := bits.LeadingZeros32(e.codIRange) - 23
	e.codIRange <<= uint(n)
	e.bits -= n
	return 0
}
