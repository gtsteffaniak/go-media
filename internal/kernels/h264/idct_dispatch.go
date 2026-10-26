package h264

func inverseTransform4x4Residual(d [16]int32) [16]int32 {
	if KernelSIMD("idct4x4") {
		return inverseTransform4x4ResidualSIMD(d)
	}
	return inverseTransform4x4ResidualPortable(d)
}
