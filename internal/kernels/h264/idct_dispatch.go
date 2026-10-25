package h264

func inverseTransform4x4Residual(d [16]int32) [16]int32 {
	if KernelsUseSIMD {
		return inverseTransform4x4ResidualSIMD(d)
	}
	return inverseTransform4x4ResidualPortable(d)
}
