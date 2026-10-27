//go:build !goexperiment.simd

package h264

func dequant4x4SIMD(coef [16]int32, qp int) [16]int32 {
	return dequant4x4Portable(coef, qp)
}
