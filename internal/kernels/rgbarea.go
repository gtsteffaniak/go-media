package kernels

import (
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"
)

// RGBAccumRow420 converts one 4:2:0 source row to RGB with lut and adds each
// pixel's R, G and B to accR/accG/accB[i]. y holds len(accR) luma samples
// starting at an even x, so luma i uses chroma i/2 from cb and cr.
// The result is identical to per-pixel yuv.Clamp8 sums over the LUT.
func RGBAccumRow420(accR, accG, accB []int32, y, cb, cr []byte, lut *yuv.RGBLUT) {
	n := len(accR)
	i := 0
	if simdRGBArea && h264k.KernelsUseSIMD {
		i = rgbAccumRow420SIMD(accR, accG, accB, y, cb, cr, lut)
	}
	rgbAccumRow420Portable(accR[i:n], accG[i:n], accB[i:n], y[i:n], cb[i/2:], cr[i/2:], lut)
}

func rgbAccumRow420Portable(accR, accG, accB []int32, y, cb, cr []byte, lut *yuv.RGBLUT) {
	accG = accG[:len(accR)]
	accB = accB[:len(accR)]
	y = y[:len(accR)]
	for i := range accR {
		yv := lut.Y[y[i]]
		c, r := cb[i>>1], cr[i>>1]
		accR[i] += yuv.Clamp8(yv + lut.CrR[r])
		accG[i] += yuv.Clamp8(yv + lut.CbG[c] + lut.CrG[r])
		accB[i] += yuv.Clamp8(yv + lut.CbB[c])
	}
}
