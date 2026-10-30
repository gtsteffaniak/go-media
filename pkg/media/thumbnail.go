package media

import (
	"fmt"
	"io"

	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/thumb"
)

// Thumbnail returns a scaled JPEG, PNG, or WebP preview of the keyframe at
// the requested seek position. Colour and keyframe selection follow ffmpeg
// (see the package doc): VUI tags honored, untagged video BT.601 limited,
// JPEG/WebP shown as BT.601, open-GOP keyframes decoded when intra or
// recovery-point; other streams return ErrUnsupported/ErrUnsupportedProfile.
func Thumbnail(r io.ReaderAt, size int64, opt ThumbnailOptions) (res ThumbnailResult, err error) {
	defer func() {
		if p := recover(); p != nil {
			res = ThumbnailResult{}
			err = panicErr(p)
		}
	}()
	out, err := thumb.Generate(r, size, mapThumbOptions(opt))
	if err != nil {
		return ThumbnailResult{}, mapErr(err)
	}
	return ThumbnailResult{
		MIME:   out.MIME,
		Data:   out.Data,
		Source: Source(out.Source),
		Width:  out.Width,
		Height: out.Height,
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

// ThumbnailRasterPNG returns a lossless PNG of the scaled decode (before
// JPEG/WebP). It converts through RGB with the stream's tagged matrix and
// range, like ffmpeg's PNG output; it skips the BT.601 JPEG/WebP rule.
func ThumbnailRasterPNG(r io.ReaderAt, size int64, opt ThumbnailOptions) (data []byte, err error) {
	defer func() {
		if p := recover(); p != nil {
			data = nil
			err = panicErr(p)
		}
	}()
	data, _, err = thumb.RasterPNG(r, size, mapThumbOptions(opt))
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
func DecodeH264Keyframe(nal []byte, width, height int) (data []byte, w, h int, err error) {
	defer func() {
		if p := recover(); p != nil {
			data = nil
			w, h = 0, 0
			err = panicErr(p)
		}
	}()
	data, w, h, err = h264.DecodeKeyframe(nal, width, height)
	if err != nil {
		return nil, 0, 0, mapErr(err)
	}
	return data, w, h, nil
}

func panicErr(p interface{}) error {
	return fmt.Errorf("media: internal error: %v", p)
}
