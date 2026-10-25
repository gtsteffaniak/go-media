package thumb

import (
	"image"

	"golang.org/x/image/draw"
)

func scaleRGBACatmull(src, dst *image.RGBA) {
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
}

// boxHalfRGBA halves an RGBA image with a 2×2 box filter.
func boxHalfRGBA(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w/2, h/2
	if nw < 1 || nh < 1 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			var pr, pg, pb, pa int
			for dy := 0; dy < 2; dy++ {
				for dx := 0; dx < 2; dx++ {
					si := src.Pix[(b.Min.Y+2*y+dy)*src.Stride+(b.Min.X+2*x+dx)*4:]
					pr += int(si[0])
					pg += int(si[1])
					pb += int(si[2])
					pa += int(si[3])
				}
			}
			di := y*dst.Stride + x*4
			dst.Pix[di+0] = byte((pr + 2) / 4)
			dst.Pix[di+1] = byte((pg + 2) / 4)
			dst.Pix[di+2] = byte((pb + 2) / 4)
			dst.Pix[di+3] = byte((pa + 2) / 4)
		}
	}
	return dst
}
