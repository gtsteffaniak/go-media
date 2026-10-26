package h264

func inverseTransform8x8Residual(d [64]int32) [64]int32 {
	if KernelSIMD("idct8x8") {
		return inverseTransform8x8ResidualSIMD(d)
	}
	return inverseTransform8x8ResidualPortable(d)
}
