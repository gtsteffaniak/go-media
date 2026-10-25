//go:build amd64 && !purego

package kernels

import "unsafe"

//go:noescape
func reconstructLuma4x4Asm(y unsafe.Pointer, stride int, px, py int, pred unsafe.Pointer, residual unsafe.Pointer)

func reconstructLuma4x4(yPlane []byte, stride int, px, py int, pred [4][4]uint8, residual [16]int32) {
	if len(yPlane) == 0 {
		return
	}
	reconstructLuma4x4Asm(unsafe.Pointer(&yPlane[0]), stride, px, py, unsafe.Pointer(&pred[0][0]), unsafe.Pointer(&residual[0]))
}
