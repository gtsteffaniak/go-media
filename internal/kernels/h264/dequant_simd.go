//go:build goexperiment.simd

package h264

import "simd"

func dequant4x4SIMD(coef [16]int32, qp int) [16]int32 {
	recordSIMD(ProbeDequant4x4)
	m := qp % 6
	shift := qp / 6
	var d [16]int32
	if shift >= 4 {
		for idx := 0; idx < 16; idx++ {
			ls := levelScale4x4(m, idx/4, idx%4)
			d[idx] = dequantCoeffAt(coef[idx], ls, shift)
		}
		return d
	}
	bias := int32(1 << uint(3-shift))
	bv := simd.BroadcastInt32s(bias)
	sh := uint(4 - shift)
	for idx := 0; idx < 16; idx += 4 {
		var ls4, c4 [4]int32
		for i := 0; i < 4; i++ {
			j := idx + i
			ls4[i] = levelScale4x4(m, j/4, j%4)
			c4[i] = coef[j]
		}
		cv, _ := simd.LoadInt32sPart(c4[:])
		lsv, _ := simd.LoadInt32sPart(ls4[:])
		sum := cv.Mul(lsv).Add(bv)
		var part [4]int32
		sum.StorePart(part[:])
		for i := 0; i < 4; i++ {
			d[idx+i] = saturateDequantCoeff(part[i] >> sh)
		}
	}
	return d
}
