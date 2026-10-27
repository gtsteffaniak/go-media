//go:build !goexperiment.simd

package h264

func inverseTransform4x4ResidualSIMD(d [16]int32) [16]int32 {
	return inverseTransform4x4ResidualPortable(d)
}
