package h264

import (
	"image"
	"image/color"

	"github.com/gtsteffaniak/go-media/internal/h264/frame"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// ThumbYCbCr is a decoded IDR frame kept in YCbCr for fast downscale before JPEG encode.
// Color metadata follows the bitstream VUI (matches ffmpeg swscale defaults).
type ThumbYCbCr struct {
	YCbCr *image.YCbCr
	CS    yuv.ColorSpace
	RNG   yuv.Range
}

// NewThumbYCbCr wraps a decoded frame for the thumbnail pipeline.
func NewThumbYCbCr(f *frame.Frame) *ThumbYCbCr {
	cs, rng := ThumbColorFromFrame(f)
	return &ThumbYCbCr{
		YCbCr: FrameToYCbCr(f),
		CS:    cs,
		RNG:   rng,
	}
}

// RGB converts limited/full-range YCbCr samples to 8-bit RGB using the stream matrix.
func (t *ThumbYCbCr) RGB(yy, cb, cr byte) (r, g, b byte) {
	ri, gi, bi := yuv.YCbCrToRGBCS(int(yy), int(cb), int(cr), t.CS, t.RNG)
	return byte(ri), byte(gi), byte(bi)
}

func (t *ThumbYCbCr) ColorModel() color.Model { return color.RGBAModel }
func (t *ThumbYCbCr) Bounds() image.Rectangle {
	if t.YCbCr == nil {
		return image.Rect(0, 0, 0, 0)
	}
	return t.YCbCr.Bounds()
}
func (t *ThumbYCbCr) At(x, y int) color.Color {
	if t.YCbCr == nil {
		return color.RGBA{}
	}
	yy := t.YCbCr.Y[t.YCbCr.YOffset(x, y)]
	cb := t.YCbCr.Cb[t.YCbCr.COffset(x, y)]
	cr := t.YCbCr.Cr[t.YCbCr.COffset(x, y)]
	r, g, b := t.RGB(yy, cb, cr)
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// RGBLUT returns lookup tables matching RGB for this frame's colour space.
func (t *ThumbYCbCr) RGBLUT() *yuv.RGBLUT { return yuv.NewRGBLUT(t.CS, t.RNG) }
