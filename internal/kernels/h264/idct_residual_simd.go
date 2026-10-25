//go:build goexperiment.simd && !purego

package h264

func inverseTransform4x4Residual(d [16]int32) [16]int32 {
	// archsimd hook: portable path is bit-identical today; swap when vector IDCT lands.
	return inverseTransform4x4ResidualPortable(d)
}
