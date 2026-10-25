package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/thumb"
)

// Thumbnail returns a scaled JPEG, PNG, or WebP preview.
func Thumbnail(r io.ReaderAt, size int64, opt ThumbnailOptions) (ThumbnailResult, error) {
	res, err := thumb.Generate(r, size, mapThumbOptions(opt))
	if err != nil {
		return ThumbnailResult{}, mapErr(err)
	}
	return ThumbnailResult{
		MIME:   res.MIME,
		Data:   res.Data,
		Source: Source(res.Source),
		Width:  res.Width,
		Height: res.Height,
	}, nil
}

// ThumbnailSmall is a convenience wrapper for the small (fast) preset.
func ThumbnailSmall(r io.ReaderAt, size int64, seekPercent float64) (ThumbnailResult, error) {
	return Thumbnail(r, size, OptionsForPreset(ThumbnailPresetSmall, seekPercent))
}

// ThumbnailLarge is a convenience wrapper for the large (high quality) preset.
func ThumbnailLarge(r io.ReaderAt, size int64, seekPercent float64) (ThumbnailResult, error) {
	return Thumbnail(r, size, OptionsForPreset(ThumbnailPresetLarge, seekPercent))
}

// ThumbnailRasterPNG returns a lossless PNG of the scaled decode (before JPEG/WebP).
func ThumbnailRasterPNG(r io.ReaderAt, size int64, opt ThumbnailOptions) ([]byte, error) {
	data, _, err := thumb.RasterPNG(r, size, mapThumbOptions(opt))
	if err != nil {
		return nil, mapErr(err)
	}
	return data, nil
}

func mapThumbOptions(opt ThumbnailOptions) thumb.Options {
	if opt.Preset != "" {
		ApplyPreset(&opt, opt.Preset)
	}
	seek := opt.SeekPercent
	if opt.Seek > 0 {
		seek = opt.Seek
	}
	topt := thumb.Options{
		MaxWidth:       opt.MaxWidth,
		MaxHeight:      opt.MaxHeight,
		SeekPercent:    seek,
		Format:         opt.Format,
		JPEGQuality:    opt.JPEGQuality,
		Quality:        opt.Quality,
		Fast:           opt.Fast,
		PreferEmbedded: opt.PreferEmbedded,
	}
	switch opt.Filter {
	case ThumbnailFilterFast:
		topt.Scaler = thumb.ScalerFast
	case ThumbnailFilterArea:
		topt.Scaler = thumb.ScalerArea
	case ThumbnailFilterBalanced:
		topt.Scaler = thumb.ScalerBalanced
	case ThumbnailFilterHigh:
		topt.Scaler = thumb.ScalerHigh
	case "":
		if opt.Fast {
			topt.Scaler = thumb.ScalerArea
		} else {
			topt.Scaler = thumb.ScalerBalanced
		}
	default:
		topt.Scaler = thumb.ScalerBalanced
	}
	return topt
}

// DecodeH264Keyframe exposes the intra-only H.264 decoder entry point.
func DecodeH264Keyframe(nal []byte, width, height int) ([]byte, int, int, error) {
	data, w, h, err := h264.DecodeKeyframe(nal, width, height)
	if err != nil {
		return nil, 0, 0, mapErr(err)
	}
	return data, w, h, nil
}
