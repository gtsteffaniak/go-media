package av1decode

import (
	"errors"
	"image"

	"github.com/gtsteffaniak/go-av1/pkg/container"
	"github.com/gtsteffaniak/go-av1/pkg/decode"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// Decoder reuses go-av1 KeyframeDecoder scratch across samples from one file.
type Decoder struct {
	kd *decode.KeyframeDecoder
}

// NewDecoderFromProbe sizes decode state from a representative temporal unit.
func NewDecoderFromProbe(probe []byte) (*Decoder, error) {
	kd, err := decode.NewKeyframeDecoder(probe,
		decode.KeyframeSkipFilmGrain(),
		decode.KeyframeOutput8Bit(),
	)
	if err != nil {
		return nil, mapErr(err)
	}
	return &Decoder{kd: kd}, nil
}

// Close releases worker goroutines held by the underlying decoder.
func (d *Decoder) Close() {
	if d == nil || d.kd == nil {
		return
	}
	d.kd.Close()
	d.kd = nil
}

// DecodeSample decodes one keyframe using optional av1C extradata and sample bytes.
// The returned picture is a copy (safe to keep after the next call) tagged with
// the sequence header's matrix_coefficients and color_range, mapped the way
// ffmpeg's swscale does (see sequenceColor).
func (d *Decoder) DecodeSample(av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	if d == nil || d.kd == nil {
		return nil, errs.UnsupProfile("av1 keyframe", "decoder closed")
	}
	cfg := ConfigOBUs(av1CExtradata)
	img, err := d.kd.DecodeSample(cfg, sample)
	if err != nil {
		return nil, mapErr(err)
	}
	th := PictureThumbYCbCr(img)
	if th != nil {
		th.CS, th.RNG = sequenceColor(cfg, sample)
	}
	return th, nil
}

// ProbeTemporalUnit builds a low-overhead temporal unit for KeyframeDecoder sizing.
func ProbeTemporalUnit(av1CExtradata, sample []byte) ([]byte, error) {
	unit, err := container.BuildTemporalUnit(ConfigOBUs(av1CExtradata), sample)
	if err != nil {
		return nil, mapErr(err)
	}
	return unit, nil
}

// DecodeSampleOnce decodes one keyframe using a cached decoder when possible.
func DecodeSampleOnce(av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	return DecodeSampleCached(av1CExtradata, sample)
}

// ConfigOBUs returns sequence-header OBUs from an av1C box or Matroska CodecPrivate payload.
func ConfigOBUs(av1CExtradata []byte) []byte {
	if len(av1CExtradata) == 0 {
		return nil
	}
	p, err := container.ParseAv1C(av1CExtradata)
	if err != nil {
		return nil
	}
	return p.ConfigOBUs
}

// PictureThumbYCbCr copies a decoded AV1 keyframe into ThumbYCbCr. The planes of
// img alias decoder memory, so the copy must be taken before the decoder is used
// again. KeyframeImage carries no color_config, so the result is tagged with the
// untagged-input default (BT.601 limited); Decoder.DecodeSample replaces that
// with the sequence header's matrix and range.
func PictureThumbYCbCr(img decode.KeyframeImage) *h264.ThumbYCbCr {
	if img.Width <= 0 || img.Height <= 0 {
		return nil
	}
	w, h := img.Width, img.Height
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	copyPlane8(ycc.Y, ycc.YStride, img.Y, img.YStride, w, h)
	if len(img.U) > 0 && len(img.V) > 0 {
		hc, wc := (h+1)/2, (w+1)/2
		copyPlane8(ycc.Cb, ycc.CStride, img.U, img.UVStride, wc, hc)
		copyPlane8(ycc.Cr, ycc.CStride, img.V, img.UVStride, wc, hc)
	} else {
		for i := range ycc.Cb {
			ycc.Cb[i] = 128
			ycc.Cr[i] = 128
		}
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.LimitedRange}
}

func copyPlane8(dst []byte, dstStride int, src []byte, srcStride, w, h int) {
	for y := 0; y < h; y++ {
		if y*srcStride+w > len(src) || y*dstStride+w > len(dst) {
			break
		}
		copy(dst[y*dstStride:y*dstStride+w], src[y*srcStride:y*srcStride+w])
	}
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, decode.ErrNotAVIFGrid) {
		return errs.Unsup("thumbnail", "avif: "+err.Error())
	}
	var ue *errs.UnsupportedError
	if errors.As(err, &ue) {
		return ue
	}
	var pe *errs.ProfileError
	if errors.As(err, &pe) {
		return pe
	}
	msg := err.Error()
	if msg == "" {
		msg = "decode failed"
	}
	return errs.UnsupProfile("av1 keyframe", msg)
}
