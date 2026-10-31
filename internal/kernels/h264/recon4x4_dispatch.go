package h264

func writeBlock4x4(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, res [16]int32) {
	if simdRecon4x4 && KernelsUseSIMD {
		writeBlock4x4SIMD(plane, stride, px0, py0, pred, predStride, lx, ly, res)
		return
	}
	writeBlock4x4Portable(plane, stride, px0, py0, pred, predStride, lx, ly, res)
}
