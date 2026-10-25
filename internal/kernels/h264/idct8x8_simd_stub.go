//go:build !goexperiment.simd || purego

package h264

func inverseTransform8x8ResidualSIMD(d [64]int32) [64]int32 {
	return inverseTransform8x8ResidualPortable(d)
}
