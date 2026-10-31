package h264

func inverseTransform8x8Residual(d [64]int32) [64]int32 {
	if simdIDCT8x8 && KernelsUseSIMD {
		return inverseTransform8x8ResidualSIMD(d)
	}
	return inverseTransform8x8ResidualPortable(d)
}
