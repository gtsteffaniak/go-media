package av1decode

import (
	"github.com/gtsteffaniak/go-av1/pkg/av1"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// sequenceColor returns the YCbCr→RGB matrix and range signalled by the AV1
// sequence header color_config. The sequence header is taken from the av1C
// configOBUs first and, when absent, from the sample itself (in-band sequence
// headers precede the first frame OBU of a keyframe temporal unit).
//
// The mapping matches ffmpeg (libdav1d/libaom → AVFrame colorspace/color_range
// → swscale, which is what ffmpeg -frames:v 1 uses for the JPEG thumbnail):
//   - matrix_coefficients 1 (BT.709) and 9/10 (BT.2020) select those matrices;
//     2 (unspecified) and every other value fall back to swscale's BT.601 default,
//     exactly like hevcdecode.ThumbColorFromPicture.
//   - color_range=1 selects full (JPEG) range, otherwise limited (MPEG) range.
//
// When no sequence header can be parsed the decoder's historic BT.601 limited
// default is kept, which is also swscale's default for untagged input.
func sequenceColor(configOBUs, sample []byte) (yuv.ColorSpace, yuv.Range) {
	if cs, rng, ok := sequenceColorFromOBUs(configOBUs); ok {
		return cs, rng
	}
	if cs, rng, ok := sequenceColorFromOBUs(sample); ok {
		return cs, rng
	}
	return yuv.BT601, yuv.LimitedRange
}

func sequenceColorFromOBUs(src []byte) (yuv.ColorSpace, yuv.Range, bool) {
	if len(src) == 0 {
		return 0, 0, false
	}
	it := av1.NewLowOverheadIterator(src)
	for {
		u, ok, err := it.Next()
		if err != nil || !ok {
			return 0, 0, false
		}
		switch u.Header.Type {
		case av1.OBUSequenceHeader:
			sh, err := av1.ParseSequenceHeader(u.Payload)
			if err != nil {
				return 0, 0, false
			}
			cs, rng := colorFromConfig(sh.ColorConfig.MatrixCoefficients, sh.ColorConfig.ColorRange)
			return cs, rng, true
		case av1.OBUFrame, av1.OBUFrameHeader, av1.OBUTileGroup:
			// A sequence header must precede frame data; stop scanning the
			// (potentially large) tile payload.
			return 0, 0, false
		}
	}
}

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

// sequenceFrameSize returns max_frame_width and max_frame_height from the
// first sequence header OBU in src.
func sequenceFrameSize(src []byte) (w, h int, ok bool) {
	if len(src) == 0 {
		return 0, 0, false
	}
	it := av1.NewLowOverheadIterator(src)
	for {
		u, more, err := it.Next()
		if err != nil || !more {
			return 0, 0, false
		}
		switch u.Header.Type {
		case av1.OBUSequenceHeader:
			sh, err := av1.ParseSequenceHeader(u.Payload)
			if err != nil {
				return 0, 0, false
			}
			return int(sh.MaxFrameWidth), int(sh.MaxFrameHeight), true
		case av1.OBUFrame, av1.OBUFrameHeader, av1.OBUTileGroup:
			return 0, 0, false
		}
	}
}
