package thumb

import (
	"image"

	"golang.org/x/image/draw"
)

func resizeDraw(src *image.RGBA, w, h int, approx bool) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if approx {
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	} else {
		draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	}
	return dst
}

func resizeCatmull(src *image.RGBA, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
