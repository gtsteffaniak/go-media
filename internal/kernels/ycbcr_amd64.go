//go:build amd64 && !purego

package kernels

import "unsafe"

// AreaDownRowY averages source luma rows [sy0,sy1) across [x0,x1) into dstY[dx].
func AreaDownRowY(dstY []byte, _ int, dx int, srcY []byte, srcStride int, sy0, sy1, x0, x1 int) {
	if dx < 0 || dx >= len(dstY) {
		return
	}
	areaDownRowYAsm(unsafe.Pointer(&dstY[dx]), unsafe.Pointer(&srcY[0]), srcStride, sy0, sy1, x0, x1)
}

// AreaDownChromaTile averages chroma in the tile covering luma [x0,x1)×[sy0,sy1).
func AreaDownChromaTile(dstCb, dstCr []byte, dstCStride int, dx, dy int, srcCb, srcCr []byte, srcCStride int, sy0, sy1, x0, x1 int) {
	areaDownChromaTilePortable(dstCb, dstCr, dstCStride, dx, dy, srcCb, srcCr, srcCStride, sy0, sy1, x0, x1)
}

// PackRGBARow writes one row of RGBA from averaged Y/Cb/Cr columns.
func PackRGBARow(dst []byte, dstStride int, dy int, yy, cb, cr []byte) {
	packRGBARowPortable(dst, dstStride, dy, yy, cb, cr)
}

//go:noescape
func areaDownRowYAsm(dstPtr, srcPtr unsafe.Pointer, srcStride, sy0, sy1, x0, x1 int)
