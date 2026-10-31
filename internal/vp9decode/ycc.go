package vp9decode

import (
	"image"

	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	"github.com/gtsteffaniak/govpx"
)

// PictureThumbYCbCr copies a decoded VP9 frame into ThumbYCbCr (BT.601 limited).
func PictureThumbYCbCr(p *govpx.Image) *h264.ThumbYCbCr {
	if p == nil || p.Width <= 0 || p.Height <= 0 {
		return nil
	}
	w, h := p.Width, p.Height
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	ys, us, vs := p.YStride, p.UStride, p.VStride
	for y := 0; y < h; y++ {
		copy(ycc.Y[y*ycc.YStride:y*ycc.YStride+w], p.Y[y*ys:y*ys+w])
		if y%2 == 0 {
			cy := y / 2
			cw := (w + 1) / 2
			copy(ycc.Cb[cy*ycc.CStride:cy*ycc.CStride+cw], p.U[(cy*us):(cy*us+cw)])
			copy(ycc.Cr[cy*ycc.CStride:cy*ycc.CStride+cw], p.V[(cy*vs):(cy*vs+cw)])
		}
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.LimitedRange}
}
