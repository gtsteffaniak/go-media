package jpegfast

import (
	"image"
	"image/color"
)

func drawYCbCr(dst *image.RGBA, r image.Rectangle, src *image.YCbCr, sp image.Point) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		sy := sp.Y + (y - r.Min.Y)
		dstRow := (y - dst.Rect.Min.Y) * dst.Stride
		for x := r.Min.X; x < r.Max.X; x++ {
			sx := sp.X + (x - r.Min.X)
			yy := src.Y[src.YOffset(sx, sy)]
			cb := src.Cb[src.COffset(sx, sy)]
			cr := src.Cr[src.COffset(sx, sy)]
			r8, g8, b8 := color.YCbCrToRGB(yy, cb, cr)
			di := dstRow + (x-dst.Rect.Min.X)*4
			dst.Pix[di+0] = r8
			dst.Pix[di+1] = g8
			dst.Pix[di+2] = b8
			dst.Pix[di+3] = 255
		}
	}
}
