//go:build !amd64 || purego

package reconstruct

func reconstructLuma4x4(yPlane []byte, stride int, px, py int, pred [4][4]uint8, residual [16]int32) {
	reconstructLuma4x4Portable(yPlane, stride, px, py, pred, residual)
}
