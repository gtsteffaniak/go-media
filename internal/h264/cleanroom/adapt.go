package cleanroom

import (
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
)

// ToFrame converts a clean-room picture to the shared frame buffer type.
// When dec is non-nil and src is dec's pooled picture with no crop offset,
// plane memory is transferred to the returned frame (zero-copy) and dec.pic is cleared.
// Cropped pictures are copied and dec keeps its buffer for reuse; that is safe
// because DecodeIDRAVC rewrites every macroblock before returning a picture and
// drops the buffer on any decode error.
func ToFrame(dec *Decoder, src *decode.Frame) *frame.Frame {
	if src == nil {
		return nil
	}
	if src.CropX == 0 && src.CropY == 0 {
		out := &frame.Frame{
			Width:    src.CropWidth,
			Height:   src.CropHeight,
			MBWidth:  src.MbWidth,
			MBHeight: src.MbHeight,
			Y:        src.Y,
			Cb:       src.Cb,
			Cr:       src.Cr,
			StrideY:  src.StrideY,
			StrideC:  src.StrideC,
		}
		copyColour(out, src)
		if dec != nil && dec.pic == src {
			dec.pic = nil
		}
		return out
	}
	return copyToFrame(src)
}

func copyToFrame(src *decode.Frame) *frame.Frame {
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
	copyColour(dst, src)
	return dst
}

// copyColour carries the SPS VUI colour signalling onto the shared frame so
// the thumbnail colour selection can honor explicit tags like ffmpeg does.
func copyColour(dst *frame.Frame, src *decode.Frame) {
	dst.VideoFullRangeFlag = src.VideoFullRange
	dst.ColorDescriptionValid = src.ColourDescription
	dst.MatrixCoefficients = uint(src.MatrixCoefficients)
}
