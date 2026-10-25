package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/flac"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/ogg"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/tagread"
	"github.com/gtsteffaniak/go-media/internal/wav"
)

// Probe inspects a media file and returns duration, streams, tags, and subtitle track list.
func Probe(r io.ReaderAt, size int64) (*ProbeInfo, error) {
	if size <= 0 {
		return nil, ErrCorrupt
	}
	id, err := sniff.Detect(r, size)
	if err != nil {
		return nil, mapErr(err)
	}
	info := &ProbeInfo{Format: toFormat(id)}
	switch id {
	case format.MP4:
		info.FormatName = isobmff.FormatName(r, size)
	case format.WebM:
		info.FormatName = "webm"
	case format.MKV:
		info.FormatName = "matroska"
	case format.WAV:
		info.FormatName = "wav"
	case format.FLAC:
		info.FormatName = "flac"
	case format.OGG:
		info.FormatName = "ogg"
	case format.MP3:
		info.FormatName = "mp3"
	default:
		info.FormatName = string(id)
	}

	if dr, err := Duration(r, size); err == nil {
		info.Duration = dr.Duration
		info.DurationSrc = dr.Source
	}

	if streams, err := streamsForFormat(r, size, id); err == nil {
		info.Streams = streams
	}

	if tags, err := tagread.ReadTags(r, size, id); err == nil {
		info.Tags = mapTags(tags)
	}
	info.HasLyrics = tagread.HasEmbeddedLyrics(r, size, id) || info.Tags.Raw["lyrics"] != ""
	info.HasArtwork = tagread.HasArtwork(r, size, id)

	subs, _ := ListSubtitleTracks(r, size)
	info.SubtitleTracks = subs

	return info, nil
}

func streamsForFormat(r io.ReaderAt, size int64, id format.ID) ([]Stream, error) {
	switch id {
	case format.MP4:
		s, err := isobmff.Streams(r, size)
		return mapStreams(s), mapErr(err)
	case format.MKV, format.WebM:
		s, err := matroska.Streams(r, size)
		return mapStreams(s), mapErr(err)
	case format.WAV:
		s, err := wav.Streams(r, size)
		return mapStreams(s), mapErr(err)
	case format.FLAC:
		s, err := flac.Streams(r, size)
		return mapStreams(s), mapErr(err)
	case format.OGG:
		s, err := ogg.Streams(r, size)
		return mapStreams(s), mapErr(err)
	default:
		return nil, Unsupported("streams", string(id))
	}
}

func mapTags(t tagread.Tags) Tags {
	return Tags{
		Title:  t.Title,
		Artist: t.Artist,
		Album:  t.Album,
		Year:   t.Year,
		Genre:  t.Genre,
		Track:  t.Track,
		Raw:    t.Raw,
	}
}
