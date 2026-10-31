package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/avi"
	"github.com/gtsteffaniak/go-media/internal/flac"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/mp3"
	"github.com/gtsteffaniak/go-media/internal/ogg"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/wav"
)

// Duration returns media length using header reads when possible.
func Duration(r io.ReaderAt, size int64) (DurationResult, error) {
	id, err := sniff.Detect(r, size)
	if err != nil {
		return DurationResult{}, mapErr(err)
	}
	switch id {
	case format.MP4:
		res, err := isobmff.Duration(r, size)
		return mapDuration(res), mapErr(err)
	case format.MKV, format.WebM:
		res, err := matroska.Duration(r, size)
		return mapDuration(res), mapErr(err)
	case format.WAV:
		res, err := wav.Duration(r, size)
		return mapDuration(res), mapErr(err)
	case format.FLAC:
		res, err := flac.Duration(r, size)
		return mapDuration(res), mapErr(err)
	case format.OGG:
		res, err := ogg.Duration(r, size)
		return mapDuration(res), mapErr(err)
	case format.MP3:
		res, err := mp3.Duration(r, size)
		return mapDuration(res), mapErr(err)
	case format.AVI:
		res, err := avi.Duration(r, size)
		return mapDuration(res), mapErr(err)
	default:
		return DurationResult{}, Unsupported("duration", string(id))
	}
}
