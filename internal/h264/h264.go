// Package h264 implements intra-only H.264/AVC keyframe decode for thumbnails.
//
// Intra-only AVC decode for thumbnails via internal/h264/cleanroom (no Eyevinn/hi264).
//
// # Keyframes and open GOP
//
// A thumbnail decodes one sync sample (MP4 stss) with an empty DPB, as
// ffmpeg's fast seek (-ss before -i) does. IDR samples always qualify. A
// non-IDR sync sample (x264 --open-gop) is decoded as a fresh start when all
// of its slices are I slices, or when it carries a recovery point SEI with
// recovery_frame_cnt 0. Any other sample, or one that still contains P/B
// slices, fails fast with errs.ErrUnsupportedProfile so callers fall back to
// ffmpeg. See cleanroom.openGOPKeyframe.
//
// # Colour
//
// Colour follows ffmpeg rather than guessing (see ThumbColorFromFrame and
// ThumbYCbCr.MatchFFmpegYUVOutput):
//   - The SPS VUI video_signal_type is honored: video_full_range_flag selects
//     full range, matrix_coefficients selects BT.709/BT.2020/BT.601. The MP4
//     colr box is not read; the bitstream VUI is what ffmpeg's decoder uses.
//   - Untagged streams (no colour description, or matrix 0/2) are BT.601
//     limited range at every resolution, swscale's default. HD is not
//     assumed to be BT.709.
//   - JPEG and WebP output use the BT.601 matrix whatever the tag says,
//     because ffmpeg's capture stays in YUV and those formats are always
//     displayed as BT.601. PNG output goes through RGB and keeps the tag.
//
// Known residual differences from ffmpeg: full-range and BT.709 sources are
// converted through an RGB intermediate that clips out-of-gamut values while
// ffmpeg passes YUV straight through (about 5 dB PSNR lower than untagged
// sources), and image/jpeg quantizes differently from ffmpeg's mjpeg encoder.
package h264

import (
	"errors"
	"fmt"
	"image"
	"strings"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/decode"
	"github.com/gtsteffaniak/go-media/internal/h264/frame"
)

// DecodeKeyframe decodes an H.264 keyframe access unit in AVCC form
// (length-prefixed NALs): an IDR, or an open-GOP intra/recovery-point
// picture (see the package doc).
func DecodeKeyframe(nal []byte, _ int, _ int) (b []byte, w, h int, err error) {
	defer recoverDecode(&err)
	img, err := DecodeAVCC(nil, nal, false)
	if err != nil {
		return nil, 0, 0, err
	}
	b, w, h = rgbaBytes(img)
	return b, w, h, nil
}

// DecodeAVCC decodes one sync sample with optional avcC parameter sets prepended.
func DecodeAVCC(avcc *mp4.AVCDecoderConfiguration, sample []byte, fast bool) (img *image.RGBA, err error) {
	defer recoverDecode(&err)
	fr, err := decodeFrame(avcc, sample, fast)
	if err != nil {
		return nil, err
	}
	return frameToRGBA(fr), nil
}

// DecodeAVCCFrame returns the decoded frame without RGB conversion.
func DecodeAVCCFrame(avcc *mp4.AVCDecoderConfiguration, sample []byte, fast bool) (fr *frame.Frame, err error) {
	defer recoverDecode(&err)
	return decodeFrame(avcc, sample, fast)
}

// decodeFrame is exported for tests that compare YUV against ffmpeg.
func decodeFrame(avcc *mp4.AVCDecoderConfiguration, sample []byte, fast bool) (*frame.Frame, error) {
	if len(sample) == 0 {
		return nil, errs.UnsupProfile("h264 keyframe", "empty sample")
	}
	data := sample
	// An SPS inside the sample (avc3, or avc1 written with repeat-headers)
	// takes precedence over avcC, as in ffmpeg where in-band parameter sets
	// replace extradata ones. The decoder already applies them in NAL order
	// after the prepended avcC sets; this only picks which SPS to validate.
	// The NAL length field width comes from avcC (lengthSizeMinusOne+1, 1, 2
	// or 4 bytes), as ffmpeg's h264_parse uses nal_length_size; 4 is assumed
	// only when there is no avcC.
	lenSize := 4
	if avcc != nil {
		lenSize = int(avcc.LengthSizeMinusOne) + 1
	}
	sps := inBandSPS(sample, lenSize)
	if sps == nil && avcc != nil && len(avcc.SequenceParameterSets) > 0 {
		sps = avcc.SequenceParameterSets[0].NALUnit
	}
	if sps == nil {
		return nil, errs.UnsupProfile("h264 keyframe", "no SPS in avcC or keyframe sample (avc3 without in-band parameter sets)")
	}
	if err := validateSPSNAL(sps); err != nil {
		return nil, err
	}
	if avcc != nil {
		data = prependParameterSets(avcc, sample)
	}
	dec := borrowDecoder(fast)
	frames, err := dec.DecodeIDRAVC(data)
	if err != nil {
		releaseDecoder(dec)
		return nil, mapDecodeErr(err)
	}
	if len(frames) == 0 {
		releaseDecoder(dec)
		return nil, errs.UnsupProfile("h264 keyframe", "no decodable keyframe picture in sample")
	}
	out := cleanroom.ToFrame(dec, frames[0])
	releaseDecoder(dec)
	return out, nil
}

func recoverDecode(err *error) {
	if p := recover(); p != nil {
		*err = fmt.Errorf("h264: internal error: %v", p)
	}
}

func mapDecodeErr(err error) error {
	if err == nil {
		return nil
	}
	// P/B slices (including in a recovery-point picture accepted by the
	// open-GOP rule) are a profile limit, not corruption: report unsupported
	// so callers fall back to ffmpeg.
	if errors.Is(err, decode.ErrInterSlice) {
		return errs.UnsupProfile("h264 keyframe", err.Error())
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

// inBandSPS returns the first SPS NAL unit in a sample whose NAL units carry
// lenSize-byte big-endian length prefixes (1 to 4, from avcC
// lengthSizeMinusOne+1), or nil. Out-of-range lenSize values fall back to 4.
// It only hops NAL length headers, so it costs a few reads per sample.
func inBandSPS(sample []byte, lenSize int) []byte {
	if lenSize < 1 || lenSize > 4 {
		lenSize = 4
	}
	for len(sample) > lenSize {
		n := 0
		for _, b := range sample[:lenSize] {
			n = n<<8 | int(b)
		}
		sample = sample[lenSize:]
		if n <= 0 || n > len(sample) {
			return nil
		}
		if sample[0]&0x1f == 7 {
			return sample[:n]
		}
		sample = sample[n:]
	}
	return nil
}
