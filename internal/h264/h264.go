// Package h264 implements intra-only H.264/AVC keyframe decode for thumbnails.
//
// Intra-only AVC decode for thumbnails via internal/h264/cleanroom (no Eyevinn/hi264).
package h264

import (
	"fmt"
	"strings"
	"image"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom"
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// DecodeKeyframe decodes an H.264 IDR access unit in AVCC form (length-prefixed NALs).
func DecodeKeyframe(nal []byte, _ int, _ int) ([]byte, int, int, error) {
	img, err := DecodeAVCC(nil, nal, false)
	if err != nil {
		return nil, 0, 0, err
	}
	b, w, h := rgbaBytes(img)
	return b, w, h, nil
}

// DecodeAVCC decodes one sync sample with optional avcC parameter sets prepended.
func DecodeAVCC(avcc *mp4.AVCDecoderConfiguration, sample []byte, fast bool) (*image.RGBA, error) {
	fr, err := decodeFrame(avcc, sample, fast)
	if err != nil {
		return nil, err
	}
	return frameToRGBA(fr), nil
}


// DecodeAVCCFrame returns the decoded frame without RGB conversion.
func DecodeAVCCFrame(avcc *mp4.AVCDecoderConfiguration, sample []byte, fast bool) (*frame.Frame, error) {
	return decodeFrame(avcc, sample, fast)
}

// decodeFrame is exported for tests that compare YUV against ffmpeg.
func decodeFrame(avcc *mp4.AVCDecoderConfiguration, sample []byte, fast bool) (*frame.Frame, error) {
	if len(sample) == 0 {
		return nil, errs.UnsupProfile("h264 keyframe", "empty sample")
	}
	data := sample
	if avcc != nil {
		if len(avcc.SequenceParameterSets) > 0 {
			if err := validateSPSNAL(avcc.SequenceParameterSets[0].NALUnit); err != nil {
				return nil, err
			}
		}
		data = prependParameterSets(avcc, sample)
	}
	dec := borrowDecoder(fast)
	frames, err := dec.DecodeIDRAVC(data)
	releaseDecoder(dec)
	if err != nil {
		return nil, mapDecodeErr(err)
	}
	if len(frames) == 0 {
		return nil, errs.UnsupProfile("h264 keyframe", "no IDR slice in sample")
	}
	return cleanroom.ToFrame(frames[0]), nil
}

func mapDecodeErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "P_Skip"), strings.Contains(msg, "reference frame"),
		strings.Contains(msg, "B slice"), strings.Contains(msg, "inter slice"):
		return errs.UnsupProfile("h264 keyframe", msg)
	case strings.Contains(msg, "MBAFF"), strings.Contains(msg, "field_pic"), strings.Contains(msg, "interlaced"):
		return errs.UnsupProfile("h264 keyframe", msg)
	default:
		return fmt.Errorf("h264 decode: %w", err)
	}
}
