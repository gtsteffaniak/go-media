package vp8decode

import (
	"image"

	gvp8 "github.com/gen2brain/vpx/vp8"
)

// PictureYCbCr copies a decoded VP8 picture into a standard YCbCr image (4:2:0, limited-range bytes).
func PictureYCbCr(p *gvp8.Picture) *image.YCbCr {
	if p == nil {
		return nil
	}
	w, h := p.Width, p.Height
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y++ {
		yOff := y * ycc.YStride
		srcY := y * p.YStride
		copy(ycc.Y[yOff:yOff+w], p.Y[srcY:srcY+w])
		if y%2 == 0 {
			cy := y / 2
			cOff := cy * ycc.CStride
			uvRow := cy * p.UVStride
			cw := (w + 1) / 2
			copy(ycc.Cb[cOff:cOff+cw], p.U[uvRow:uvRow+cw])
			copy(ycc.Cr[cOff:cOff+cw], p.V[uvRow:uvRow+cw])
		}
	}
	return ycc
}
