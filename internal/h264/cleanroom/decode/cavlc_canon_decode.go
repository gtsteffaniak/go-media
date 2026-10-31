package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// Canonical-codeword CAVLC decoding. Tables in cavlc_canon.go are ISO/IEC 14496-10 VLCs;
// cavlc_lut.go expands them into 16-bit prefix LUTs (PeekBits + Skip) at init.

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

var (
	ctLUTNC0, ctLUTNC2, ctLUTNC4, ctLUTChromaDC [1 << cavlcPeekBits]ctSlot
	tzLUTs                                      [16][1 << cavlcPeekBits]vlcSlot
	tzChromaLUTs                                [4][1 << cavlcPeekBits]vlcSlot
	rbLUTs                                      [7][1 << cavlcPeekBits]vlcSlot
)

func init() {
	ctLUTNC0 = buildCTLUT(coeffTokenNC0)
	ctLUTNC2 = buildCTLUT(coeffTokenNC2)
	ctLUTNC4 = buildCTLUT(coeffTokenNC4)
	ctLUTChromaDC = buildCTLUT(coeffTokenChromaDC)
	for i := range totalZerosTab {
		tzLUTs[i] = buildVLCLUT(totalZerosTab[i])
	}
	for i := range totalZerosChromaTab {
		tzChromaLUTs[i] = buildVLCLUT(totalZerosChromaTab[i])
	}
	for i := range runBeforeCanon {
		rbLUTs[i] = buildVLCLUT(runBeforeCanon[i])
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
	var lut *[1 << cavlcPeekBits]ctSlot
	switch {
	case nC == -1:
		lut = &ctLUTChromaDC
	case nC < 2:
		lut = &ctLUTNC0
	case nC < 4:
		lut = &ctLUTNC2
	default:
		lut = &ctLUTNC4
	}
	return decodeCoeffTokenLUT(r, lut)
}

func decodeVLC(r *bitstream.Reader, lut *[1 << cavlcPeekBits]vlcSlot) int {
	return decodeVLCLUT(r, lut)
}

func decodeTotalZeros(r *bitstream.Reader, totalCoeff int) int {
	return decodeVLCSmall(r, tzSmall[totalCoeff][:], tzPeekBits)
}

func decodeTotalZerosChromaDC(r *bitstream.Reader, totalCoeff int) int {
	return decodeVLCSmall(r, tzChromaSmall[totalCoeff][:], rbPeekBits)
}

func decodeRunBefore(r *bitstream.Reader, zerosLeft int) int {
	if zerosLeft <= 6 {
		return decodeVLCSmall(r, rbSmall[zerosLeft][:], rbPeekBits)
	}
	// zerosLeft > 6: 3 bits; if nonzero → 7-temp, else a unary tail.
	if temp := int(r.U(3)); temp != 0 {
		return 7 - temp
	}
	return 7 + readLevelPrefix(r)
}
