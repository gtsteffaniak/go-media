//go:build !goexperiment.simd

package h264

func writeBlock4x4SIMD(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, res [16]int32) {
	writeBlock4x4Portable(plane, stride, px0, py0, pred, predStride, lx, ly, res)
}
