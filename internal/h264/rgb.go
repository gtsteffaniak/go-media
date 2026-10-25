package h264

import (
	"image"
	"image/color"

	"github.com/Eyevinn/hi264/pkg/frame"
	"github.com/Eyevinn/hi264/pkg/yuv"
)

func frameToRGBA(f *frame.Frame) *image.RGBA {
	// Match Eyevinn/hi264 FrameToImageCS defaults (BT.601 limited) when VUI is absent.
	cs := yuv.BT601
	rng := yuv.LimitedRange
	if f.ColorDescriptionValid && f.MatrixCoefficients != 0 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(f.MatrixCoefficients)
	}
	if f.VideoFullRangeFlag {
		rng = yuv.FullRange
	}
	nr := yuv.FrameToImageCS(f, cs, rng)
	img := image.NewRGBA(nr.Bounds())
	for y := nr.Rect.Min.Y; y < nr.Rect.Max.Y; y++ {
		for x := nr.Rect.Min.X; x < nr.Rect.Max.X; x++ {
			i := nr.PixOffset(x, y)
			img.Set(x, y, color.RGBA{
				R: nr.Pix[i+0],
				G: nr.Pix[i+1],
				B: nr.Pix[i+2],
				A: 255,
			})
		}
	}
	return img
}

func rgbaBytes(img *image.RGBA) ([]byte, int, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := (y*w + x) * 4
			c := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
			out[off+0] = c.R
			out[off+1] = c.G
			out[off+2] = c.B
			out[off+3] = 255
		}
	}
	return out, w, h
}
