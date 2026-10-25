//go:build !goexperiment.simd || purego

package h264

func inverseTransform4x4Residual(d [16]int32) [16]int32 {
	return inverseTransform4x4ResidualPortable(d)
}
