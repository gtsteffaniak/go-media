package av1decode

import (
	"errors"
	"fmt"
	"image"

	goav1 "github.com/gtsteffaniak/go-av1"
	"github.com/gtsteffaniak/go-av1/container"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// Decoder reuses go-av1 KeyframeDecoder scratch across samples from one file.
type Decoder struct {
	kd *goav1.KeyframeDecoder
}

// NewDecoderFromProbe sizes decode state from a representative temporal unit.
func NewDecoderFromProbe(probe []byte) (*Decoder, error) {
	kd, err := goav1.NewKeyframeDecoder(probe,
		goav1.KeyframeSkipFilmGrain(),
		goav1.KeyframeOutput8Bit(),
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
func (d *Decoder) DecodeSample(av1CExtradata, sample []byte) (*h264.ThumbYCbCr, error) {
	if d == nil || d.kd == nil {
		return nil, errs.UnsupProfile("av1 keyframe", "decoder closed")
	}
	img, err := d.kd.DecodeSample(ConfigOBUs(av1CExtradata), sample)
	if err != nil {
		return nil, mapErr(err)
	}
	return PictureThumbYCbCr(img), nil
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

// PictureThumbYCbCr copies a decoded AV1 keyframe into ThumbYCbCr.
func PictureThumbYCbCr(img goav1.KeyframeImage) *h264.ThumbYCbCr {
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
	if errors.Is(err, goav1.ErrNotAVIFGrid) {
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

// DecodeAVIFFile decodes the primary AVIF item (still or grid) from a full file buffer.
func DecodeAVIFFile(data []byte) (*h264.ThumbYCbCr, error) {
	primary, err := container.ParseAVIF(data)
	if err != nil {
		return nil, mapErr(fmt.Errorf("avif: %w", err))
	}
	if primary.IsGrid {
		return decodeAVIFGrid(data)
	}
	unit, err := container.TemporalUnitFromAVIF(primary)
	if err != nil {
		return nil, mapErr(err)
	}
	dec, err := NewDecoderFromProbe(unit)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	img, err := dec.kd.DecodeTemporalUnit(unit)
	if err != nil {
		return nil, mapErr(err)
	}
	return PictureThumbYCbCr(img), nil
}

func decodeAVIFGrid(data []byte) (*h264.ThumbYCbCr, error) {
	grid, err := goav1.ParseAVIFGrid(data)
	if err != nil {
		return nil, mapErr(err)
	}
	probe := grid.TileUnits[0]
	dec, err := NewDecoderFromProbe(probe)
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	img, err := goav1.DecodeAVIFGridKeyframe(dec.kd, data)
	if err != nil {
		return nil, mapErr(err)
	}
	return PictureThumbYCbCr(img), nil
}
