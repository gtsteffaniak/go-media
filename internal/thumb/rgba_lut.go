package thumb

import (
	"image"

	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// rgbaFromThumbLUT converts video YCbCr to RGBA using the stream's RGBLUT.
// yuv.TestRGBLUTMatchesYCbCrToRGBCS guarantees the LUT sums equal
// YCbCrToRGBCS, so the output is bit-identical to rgbaFromYCbCr with
// ThumbYCbCr.RGB, without a per-pixel closure call or SetRGBA.
func rgbaFromThumbLUT(ycc *image.YCbCr, lut *yuv.RGBLUT) *image.RGBA {
	b := ycc.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		di := (y - b.Min.Y) * dst.Stride
		for x := b.Min.X; x < b.Max.X; x++ {
			yy := lut.Y[ycc.Y[ycc.YOffset(x, y)]]
			co := ycc.COffset(x, y)
			cb := ycc.Cb[co]
			cr := ycc.Cr[co]
			o := di + 4*(x-b.Min.X)
			dst.Pix[o+0] = byte(yuv.Clamp8(yy + lut.CrR[cr]))
			dst.Pix[o+1] = byte(yuv.Clamp8(yy + lut.CbG[cb] + lut.CrG[cr]))
			dst.Pix[o+2] = byte(yuv.Clamp8(yy + lut.CbB[cb]))
			dst.Pix[o+3] = 255
		}
	}
	return dst
}
