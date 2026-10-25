package thumb

import (
	"bytes"
	"image"
	"image/jpeg"

	"github.com/gtsteffaniak/go-media/internal/jpegfast"
)

// jpegDecode decodes MJPEG/cover JPEG with DCT-truncated paths when downscaling.
func jpegDecode(data []byte, maxW, maxH int, fast bool) (image.Image, string, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Decode(bytes.NewReader(data))
	}
	tw, th := fitThumbSize(cfg.Width, cfg.Height, maxW, maxH)
	if tw <= 0 || th <= 0 {
		tw, th = cfg.Width, cfg.Height
	}
	scale := float64(cfg.Width) / float64(tw)
	if sh := float64(cfg.Height) / float64(th); sh > scale {
		scale = sh
	}
	if fast && scale >= 4 {
		trunc := 21
		if scale >= 6 {
			trunc = 40
		}
		img, err := jpegfast.DecodeTrunc(bytes.NewReader(data), trunc)
		if err == nil && img != nil {
			return img, "jpeg", nil
		}
	}
	if scale >= 1.5 && !(fast && scale >= 4) {
		trunc := 21
		if !fast && scale < 2.5 {
			trunc = 63
		}
		img, err := jpegfast.DecodeTrunc(bytes.NewReader(data), trunc)
		if err == nil && img != nil {
			return img, "jpeg", nil
		}
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return image.Decode(bytes.NewReader(data))
	}
	return img, "jpeg", nil
}
