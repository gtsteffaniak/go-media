package mpeg4

import "sync"

// tcoefEntry is one row of Table B-16: code (without sign bit), length,
// last, run and level.
type tcoefEntry struct {
	code  uint32
	len   uint8
	last  uint8
	run   uint8
	level uint16
}

// lutEntry is a decoded VLC: code length (0 = invalid code) and value.
type lutEntry struct {
	len uint8
	val int16
}

// tcoefLUT entries pack last/run/level; escape is flagged with run=255.
type tcoefLUTEntry struct {
	len   uint8
	last  uint8
	run   uint8
	level uint8
}

const (
	tcoefBits  = 12
	escapeRun  = 255
	escapeCode = 0b0000011
	escapeLen  = 7
)

var (
	tablesOnce sync.Once
	tcoefLUT   [1 << tcoefBits]tcoefLUTEntry
	// maxLevel[last][run] and maxRun[last][level] are LMAX and RMAX of
	// §7.4.1.3, derived from the intra table.
	maxLevel   [2][64]uint8
	maxRun     [2][32]uint8
	mcbpcLUT   [1 << 9]lutEntry
	cbpyLUT    [1 << 6]lutEntry
	dcLumaLUT  [1 << 11]lutEntry
	dcChromaLU [1 << 12]lutEntry
)

// mcbpcI is Table B-6 (I-VOP mcbpc): value = mb_type<<2 | cbpc; -1 = stuffing.
var mcbpcI = []struct {
	code uint32
	len  uint8
	val  int16
}{
	{0b1, 1, 3<<2 | 0}, {0b001, 3, 3<<2 | 1}, {0b010, 3, 3<<2 | 2}, {0b011, 3, 3<<2 | 3},
	{0b0001, 4, 4<<2 | 0}, {0b000001, 6, 4<<2 | 1}, {0b000010, 6, 4<<2 | 2}, {0b000011, 6, 4<<2 | 3},
	{0b000000001, 9, -1},
}

// cbpyIntra is Table B-8 (code, length) for intra cbpy values 0..15.
var cbpyIntra = [16][2]uint8{
	{3, 4}, {5, 5}, {4, 5}, {9, 4}, {3, 5}, {7, 4}, {2, 6}, {11, 4},
	{2, 5}, {3, 6}, {5, 4}, {10, 4}, {4, 4}, {8, 4}, {6, 4}, {3, 2},
}

// dcSizeLuma and dcSizeChroma are Tables B-13 and B-14 (code, length) for
// dct_dc_size 0..12.
var dcSizeLuma = [13][2]uint16{
	{0b011, 3}, {0b11, 2}, {0b10, 2}, {0b010, 3}, {0b001, 3}, {1, 4}, {1, 5},
	{1, 6}, {1, 7}, {1, 8}, {1, 9}, {1, 10}, {1, 11},
}

var dcSizeChroma = [13][2]uint16{
	{0b11, 2}, {0b10, 2}, {0b01, 2}, {1, 3}, {1, 4}, {1, 5}, {1, 6},
	{1, 7}, {1, 8}, {1, 9}, {1, 10}, {1, 11}, {1, 12},
}

func fillLUT(lut []lutEntry, bits int, code uint32, n uint8, val int16) {
	shift := bits - int(n)
	base := int(code) << shift
	for i := 0; i < 1<<shift; i++ {
		lut[base+i] = lutEntry{len: n, val: val}
	}
}

func initTables() {
	for _, e := range tcoefIntra {
		shift := tcoefBits - int(e.len)
		base := int(e.code) << shift
		for i := 0; i < 1<<shift; i++ {
			tcoefLUT[base+i] = tcoefLUTEntry{len: e.len, last: e.last, run: e.run, level: uint8(e.level)}
		}
		if e.level > uint16(maxLevel[e.last][e.run]) {
			maxLevel[e.last][e.run] = uint8(e.level)
		}
		if e.run > maxRun[e.last][e.level] {
			maxRun[e.last][e.level] = e.run
		}
	}
	shift := tcoefBits - escapeLen
	for i := 0; i < 1<<shift; i++ {
		tcoefLUT[escapeCode<<shift+i] = tcoefLUTEntry{len: escapeLen, run: escapeRun}
	}
	for _, e := range mcbpcI {
		fillLUT(mcbpcLUT[:], 9, e.code, e.len, e.val)
	}
	for v, e := range cbpyIntra {
		fillLUT(cbpyLUT[:], 6, uint32(e[0]), e[1], int16(v))
	}
	for s, e := range dcSizeLuma {
		fillLUT(dcLumaLUT[:], 11, uint32(e[0]), uint8(e[1]), int16(s))
	}
	for s, e := range dcSizeChroma {
		fillLUT(dcChromaLU[:], 12, uint32(e[0]), uint8(e[1]), int16(s))
	}
}

// readLUT decodes one VLC; ok is false for an invalid code.
func (r *reader) readLUT(lut []lutEntry, bits int) (int, bool) {
	e := lut[r.peek(bits)]
	if e.len == 0 {
		return 0, false
	}
	r.skip(int(e.len))
	return int(e.val), true
}
