package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// CABAC arithmetic decoding engine (ISO/IEC 14496-10). Standard-spec variant:
// 9-bit codIOffset, codIRange in [256,510], bit-by-bit renormalization. The
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
type cabacEngine struct {
	r          *bitstream.Reader
	codIRange  uint32
	codIOffset uint32
	ctx        [numCabacCtx]cabacCtx
}

// resetCabacEngine initializes registers and context models for a new slice.
// The RBSP must already be byte aligned (cabac_alignment_one_bit consumed).
func (e *cabacEngine) resetCabacEngine(r *bitstream.Reader, sliceQP, model int) {
	e.r = r
	e.codIRange = 510
	e.codIOffset = r.U(9)
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
func (e *cabacEngine) decodeDecision(ctxIdx int) uint32 {
	c := &e.ctx[ctxIdx]
	rangeLPS := uint32(rangeTabLPS[c.state][(e.codIRange>>6)&3])
	e.codIRange -= rangeLPS

	var binVal uint32
	if e.codIOffset >= e.codIRange {
		binVal = uint32(1 - c.mps)
		e.codIOffset -= e.codIRange
		e.codIRange = rangeLPS
		if c.state == 0 {
			c.mps = 1 - c.mps
		}
		c.state = stateTrans[c.state][0]
	} else {
		binVal = uint32(c.mps)
		c.state = stateTrans[c.state][1]
	}
	// Inlined renorm (hot path — millions of calls per frame).
	r := e.r
	for e.codIRange < 256 {
		e.codIRange <<= 1
		e.codIOffset = (e.codIOffset << 1) | r.Bit()
	}
	return binVal
}

// renorm rescales codIRange back into [256,510], pulling in fresh offset bits.
func (e *cabacEngine) renorm() {
	r := e.r
	for e.codIRange < 256 {
		e.codIRange <<= 1
		e.codIOffset = (e.codIOffset << 1) | r.Bit()
	}
}

// decodeBypass decodes one bypass (equiprobable) bin — no context, no renorm
// of codIRange.
func (e *cabacEngine) decodeBypass() uint32 {
	e.codIOffset = (e.codIOffset << 1) | e.r.Bit()
	if e.codIOffset >= e.codIRange {
		e.codIOffset -= e.codIRange
		return 1
	}
	return 0
}

// decodeTerminate decodes the end_of_slice_flag / I_PCM terminator. Returns 1
// when decoding terminates.
func (e *cabacEngine) decodeTerminate() uint32 {
	e.codIRange -= 2
	if e.codIOffset >= e.codIRange {
		return 1
	}
	e.renorm()
	return 0
}
