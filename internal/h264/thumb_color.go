package h264

import (
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// ThumbColorFromFrame returns the YCbCr→RGB matrix and range ffmpeg's swscale
// would use when converting this frame to RGB (for example PNG output).
//
// Matrix: an explicit VUI matrix_coefficients tag is honored, as swscale does
// via the frame's colorspace. Untagged streams (no colour description, or
// matrix_coefficients 2 "unspecified") use BT.601 regardless of resolution:
// swscale's default table (SWS_CS_DEFAULT) is ITU-R BT.601 and ffmpeg does
// not guess BT.709 for HD. Value 0 (identity/GBR) is not a YCbCr matrix and is
// treated as untagged as well.
//
// Range: video_full_range_flag selects full range (ffmpeg AVCOL_RANGE_JPEG);
// otherwise limited range, swscale's default for YUV input.
//
// JPEG and WebP thumbnails apply one more ffmpeg rule on top of this; see
// ThumbYCbCr.MatchFFmpegYUVOutput.
func ThumbColorFromFrame(f *frame.Frame) (yuv.ColorSpace, yuv.Range) {
	cs := yuv.BT601
	rng := yuv.LimitedRange
	if f.ColorDescriptionValid && f.MatrixCoefficients != 0 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(f.MatrixCoefficients)
	}
	if f.VideoFullRangeFlag {
		rng = yuv.FullRange
	}
	return cs, rng
}
