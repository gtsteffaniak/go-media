package h264

import (
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// ThumbColorFromFrame returns YCbCr→RGB metadata for thumbnails (matches ffmpeg when VUI is absent).
func ThumbColorFromFrame(f *frame.Frame) (yuv.ColorSpace, yuv.Range) {
	cs := yuv.BT601
	rng := yuv.LimitedRange
	if f.ColorDescriptionValid && f.MatrixCoefficients != 0 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(f.MatrixCoefficients)
	} else if f.Width >= 1280 || f.Height >= 720 {
		// Unspecified matrix on HD: libavcodec/swscale default to BT.709.
		cs = yuv.BT709
	}
	if f.VideoFullRangeFlag {
		rng = yuv.FullRange
	}
	return cs, rng
}
