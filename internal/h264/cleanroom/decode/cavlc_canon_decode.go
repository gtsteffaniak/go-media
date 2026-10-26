package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// Canonical-codeword CAVLC decoding. The codeword tables in cavlc_canon.go are
// the ISO/IEC 14496-10 variable-length codes; here they are turned into prefix
// lookup maps at init and decoded by reading one bit at a time.

// ctEntry is one coeff_token codeword: {length, code, TrailingOnes, TotalCoeff}.
type ctEntry struct {
	length, code uint16
	t0, tc       int8
}

// vlcEntry is one total_zeros / run_before codeword: {length, code, value}.
type vlcEntry struct {
	length, code uint16
	val          int8
}

// vlcKey packs (length, code) into a unique prefix-free key.
func vlcKey(length, code int) uint32 { return uint32(length)<<16 | uint32(code) }

var (
	ctMapNC0, ctMapNC2, ctMapNC4, ctMapChromaDC map[uint32][2]int8
	tzMaps                                      [16]map[uint32]int8
	tzChromaMaps                                [4]map[uint32]int8
	rbMaps                                      [7]map[uint32]int8
)

func buildCtMap(entries []ctEntry) map[uint32][2]int8 {
	m := make(map[uint32][2]int8, len(entries))
	for _, e := range entries {
		m[vlcKey(int(e.length), int(e.code))] = [2]int8{e.t0, e.tc}
	}
	return m
}

func buildVlcMap(entries []vlcEntry) map[uint32]int8 {
	m := make(map[uint32]int8, len(entries))
	for _, e := range entries {
		m[vlcKey(int(e.length), int(e.code))] = e.val
	}
	return m
}

func init() {
	ctMapNC0 = buildCtMap(coeffTokenNC0)
	ctMapNC2 = buildCtMap(coeffTokenNC2)
	ctMapNC4 = buildCtMap(coeffTokenNC4)
	ctMapChromaDC = buildCtMap(coeffTokenChromaDC)
	for i := range totalZerosTab {
		tzMaps[i] = buildVlcMap(totalZerosTab[i])
	}
	for i := range totalZerosChromaTab {
		tzChromaMaps[i] = buildVlcMap(totalZerosChromaTab[i])
	}
	for i := range runBeforeCanon {
		rbMaps[i] = buildVlcMap(runBeforeCanon[i])
	}
}

// decodeCoeffToken reads coeff_token and returns (TrailingOnes, TotalCoeff).
func decodeCoeffToken(r *bitstream.Reader, nC int) (int, int) {
	if nC >= 8 { // 6-bit fixed-length code
		code := r.U(6)
		t0, tc := int(code&3), int(code>>2)+1
		if code == 3 {
			t0, tc = 0, 0
		}
		return t0, tc
	}
	var m map[uint32][2]int8
	switch {
	case nC == -1:
		m = ctMapChromaDC
	case nC < 2:
		m = ctMapNC0
	case nC < 4:
		m = ctMapNC2
	default:
		m = ctMapNC4
	}
	code := 0
	for length := 1; length <= 16; length++ {
		code = code<<1 | int(r.Bit())
		if e, ok := m[vlcKey(length, code)]; ok {
			return int(e[0]), int(e[1])
		}
	}
	return 0, 0
}

func decodeVLC(r *bitstream.Reader, m map[uint32]int8) int {
	code := 0
	for length := 1; length <= 12; length++ {
		code = code<<1 | int(r.Bit())
		if v, ok := m[vlcKey(length, code)]; ok {
			return int(v)
		}
	}
	return 0
}

func decodeTotalZeros(r *bitstream.Reader, totalCoeff int) int {
	return decodeVLC(r, tzMaps[totalCoeff])
}

func decodeTotalZerosChromaDC(r *bitstream.Reader, totalCoeff int) int {
	return decodeVLC(r, tzChromaMaps[totalCoeff])
}

func decodeRunBefore(r *bitstream.Reader, zerosLeft int) int {
	if zerosLeft <= 6 {
		return decodeVLC(r, rbMaps[zerosLeft])
	}
	// zerosLeft > 6: 3 bits; if nonzero → 7-temp, else a unary tail.
	if temp := int(r.U(3)); temp != 0 {
		return 7 - temp
	}
	return 7 + readLevelPrefix(r)
}
