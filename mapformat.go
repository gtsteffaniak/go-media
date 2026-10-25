package media

import (
	"errors"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
)

func toFormat(id format.ID) Format {
	return Format(id)
}

func fromFormat(f Format) format.ID {
	return format.ID(f)
}

func mapDuration(d detect.DurationResult) DurationResult {
	return DurationResult{Duration: d.Duration, Source: Source(d.Source)}
}

func mapStreams(in []detect.Stream) []Stream {
	out := make([]Stream, len(in))
	for i, s := range in {
		out[i] = Stream{
			Index:      s.Index,
			Type:       StreamType(s.Type),
			Codec:      s.Codec,
			Width:      s.Width,
			Height:     s.Height,
			FrameRate:  s.FrameRate,
			SampleRate: s.SampleRate,
			Channels:   s.Channels,
			Bitrate:    s.Bitrate,
			Language:   s.Language,
			Title:      s.Title,
		}
	}
	return out
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var u *errs.UnsupportedError
	if errors.As(err, &u) {
		return Unsupported(u.Op, u.Reason)
	}
	if errors.Is(err, errs.Unsupported) {
		return ErrUnsupported
	}
	if errors.Is(err, errs.Corrupt) {
		return ErrCorrupt
	}
	if errors.Is(err, errs.NotMedia) {
		return ErrNotMedia
	}
	return err
}
