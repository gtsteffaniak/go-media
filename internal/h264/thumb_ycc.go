package h264

import (
	"image"
	"image/color"

	"github.com/Eyevinn/hi264/pkg/frame"
	"github.com/Eyevinn/hi264/pkg/yuv"
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
	cs := yuv.BT601
	rng := yuv.LimitedRange
	if f.ColorDescriptionValid && f.MatrixCoefficients != 0 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(f.MatrixCoefficients)
	}
	if f.VideoFullRangeFlag {
		rng = yuv.FullRange
	}
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

func (t *ThumbYCbCr) ColorModel() color.Model { return t.YCbCr.ColorModel() }
func (t *ThumbYCbCr) Bounds() image.Rectangle { return t.YCbCr.Bounds() }
func (t *ThumbYCbCr) At(x, y int) color.Color { return t.YCbCr.At(x, y) }
