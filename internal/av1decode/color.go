package av1decode

import (
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// colorFromConfig maps the sequence header color_config (matrix_coefficients
// and color_range, as gav1d reports them on each Picture) to the swscale matrix
// and range, matching ffmpeg (libdav1d → AVFrame colorspace/color_range →
// swscale, which is what ffmpeg -frames:v 1 uses for the JPEG thumbnail):
//   - matrix_coefficients 1 (BT.709) and 9/10 (BT.2020) select those matrices;
//     2 (unspecified) and every other value fall back to swscale's BT.601 default,
//     exactly like hevcdecode.ThumbColorFromPicture.
//   - color_range=1 selects full (JPEG) range, otherwise limited (MPEG) range.
func colorFromConfig(matrix uint8, fullRange bool) (yuv.ColorSpace, yuv.Range) {
	cs := yuv.BT601
	if matrix != 0 && matrix != 2 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(uint(matrix))
	}
	rng := yuv.LimitedRange
	if fullRange {
		rng = yuv.FullRange
	}
	return cs, rng
}
