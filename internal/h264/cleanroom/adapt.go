package cleanroom

import (
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
)

// ToFrame converts a clean-room picture to the shared frame buffer type.
func ToFrame(src *decode.Frame) *frame.Frame {
	if src == nil {
		return nil
	}
	w, h := src.CropWidth, src.CropHeight
	dst := frame.NewFrame(w, h)
	for y := 0; y < h; y++ {
		ys := (src.CropY + y) * src.StrideY
		copy(dst.Y[y*dst.StrideY:], src.Y[ys+src.CropX:ys+src.CropX+w])
	}
	ch := h / 2
	cw := w / 2
	subW, subH := 2, 2
	cy0, cx0 := src.CropY/subH, src.CropX/subW
	for y := 0; y < ch; y++ {
		cs := (cy0 + y) * src.StrideC
		copy(dst.Cb[y*dst.StrideC:], src.Cb[cs+cx0:cs+cx0+cw])
		copy(dst.Cr[y*dst.StrideC:], src.Cr[cs+cx0:cs+cx0+cw])
	}
	return dst
}
