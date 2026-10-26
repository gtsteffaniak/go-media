package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// mapCBP — mapping codeNum→coded_block_pattern (ChromaArrayType 1/2) from the
// ISO/IEC 14496-10 specification. Column 0 — Intra (Intra_4x4/8x8), column 1 —
// Inter. CBP: low 4 bits — CBPLuma, high bits — CBPChroma.
var mapCBP = [48][2]uint8{
	{47, 0}, {31, 16}, {15, 1}, {0, 2}, {23, 4}, {27, 8}, {29, 32}, {30, 3}, {7, 5}, {11, 10}, {13, 12}, {14, 15},
	{39, 47}, {43, 7}, {45, 11}, {46, 13}, {16, 14}, {3, 6}, {5, 9}, {10, 31}, {12, 35}, {19, 37}, {21, 42}, {26, 44},
	{28, 33}, {35, 34}, {37, 36}, {42, 40}, {44, 39}, {1, 43}, {2, 45}, {4, 46}, {8, 17}, {17, 18}, {18, 20}, {20, 24},
	{24, 19}, {6, 21}, {9, 26}, {22, 28}, {25, 23}, {32, 27}, {33, 29}, {34, 30}, {36, 22}, {40, 25}, {38, 38}, {41, 41},
}

// readCBP reads coded_block_pattern (me(v)) and returns CBPLuma (0..15) and
// CBPChroma (0..2). intra=true selects the Intra column.
func readCBP(r *bitstream.Reader, intra bool) (cbpLuma, cbpChroma int, ok bool) {
	codeNum := r.UE()
	if codeNum > 47 {
		return 0, 0, false
	}
	col := 1
	if intra {
		col = 0
	}
	cbp := int(mapCBP[codeNum][col])
	return cbp & 0xF, cbp >> 4, true
}
