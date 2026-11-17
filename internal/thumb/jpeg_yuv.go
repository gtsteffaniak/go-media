package thumb

import (
	"image"
	"math"

	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// fitsWithin reports whether a bounds rectangle needs no downscale for maxW×maxH,
// using the same rule as tryScaleYCbCrSource.
func fitsWithin(b image.Rectangle, maxW, maxH int) bool {
	tw, th := fitThumbSize(b.Dx(), b.Dy(), maxW, maxH)
	return tw >= b.Dx() && th >= b.Dy()
}

// jpegLumaExpand and jpegChromaExpand map limited-range samples to full range
// (Y 16..235 → 0..255, C 16..240 → 0..255) with rounding, the same mapping the
// RGB path applies when it converts limited-range YCbCr.
var jpegLumaExpand, jpegChromaExpand [256]uint8

func init() {
	for i := range jpegLumaExpand {
		jpegLumaExpand[i] = clampLUT(int(math.Floor(float64(i-16)*255/219 + 0.5)))
		jpegChromaExpand[i] = clampLUT(int(math.Floor(float64(i-128)*255/224 + 128.5)))
	}
}

func clampLUT(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// jpegReadyYCbCr returns the planes ready for image/jpeg, which encodes full-range
// BT.601 YCbCr. The matrix is already BT.601 (MatchFFmpegYUVOutput). Limited-range
// samples are expanded in place, so the RGB round trip is not needed. The expansion
// is a per-sample table on YUV planes, so it costs far less than RGB conversion.
// The ThumbYCbCr is one-shot decoder output, so mutating it in place is safe.
func jpegReadyYCbCr(t *h264.ThumbYCbCr) *image.YCbCr {
	if t.RNG == yuv.LimitedRange {
		expandPlane(t.YCbCr.Y, t.YCbCr.YStride, t.YCbCr.Rect.Dx(), t.YCbCr.Rect.Dy(), &jpegLumaExpand)
		cw, ch := chromaSize(t.YCbCr)
		expandPlane(t.YCbCr.Cb, t.YCbCr.CStride, cw, ch, &jpegChromaExpand)
		expandPlane(t.YCbCr.Cr, t.YCbCr.CStride, cw, ch, &jpegChromaExpand)
		t.RNG = yuv.FullRange
	}
	return t.YCbCr
}

func expandPlane(pix []uint8, stride, w, h int, lut *[256]uint8) {
	for y := 0; y < h; y++ {
		row := pix[y*stride : y*stride+w]
		for x, v := range row {
			row[x] = lut[v]
		}
	}
}

// chromaSize returns the chroma plane width and height for a YCbCr image.
func chromaSize(ycc *image.YCbCr) (int, int) {
	b := ycc.Rect
	switch ycc.SubsampleRatio {
	case image.YCbCrSubsampleRatio444:
		return b.Dx(), b.Dy()
	case image.YCbCrSubsampleRatio422:
		return (b.Max.X+1)/2 - b.Min.X/2, b.Dy()
	case image.YCbCrSubsampleRatio440:
		return b.Dx(), (b.Max.Y+1)/2 - b.Min.Y/2
	default: // 4:2:0
		return (b.Max.X+1)/2 - b.Min.X/2, (b.Max.Y+1)/2 - b.Min.Y/2
	}
}
