package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/asf"
	"github.com/gtsteffaniak/go-media/internal/avi"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/flac"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/mp3"
	ps "github.com/gtsteffaniak/go-media/internal/mpegps"
	ts "github.com/gtsteffaniak/go-media/internal/mpegts"
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
	if id == format.MP4 {
		return probeMP4(r, size, info)
	}
	return probeOther(r, size, id, info)
}

func probeMP4(r io.ReaderAt, size int64, info *ProbeInfo) (*ProbeInfo, error) {
	fm, err := isobmff.OpenFileMeta(r, size)
	if err != nil {
		return nil, mapErr(err)
	}
	info.FormatName = fm.FormatName()
	if dr, err := fm.Duration(); err == nil {
		mapped := mapDuration(dr)
		info.Duration = mapped.Duration
		info.DurationSrc = mapped.Source
	}
	info.Streams = mapStreams(fm.Streams())
	subs := fm.SubtitleStreams()
	info.HasArtwork = fm.HasAttachedCover()
	// Video-only MP4 (e.g. MJPEG) often has no ilst; skip dhowden/tag when nothing needs it.
	if info.HasArtwork || len(subs) > 0 {
		if tags, hasArt, hasLyrics, err := tagread.InspectTags(r, size, format.MP4); err == nil {
			info.Tags = mapTags(tags)
			info.HasLyrics = hasLyrics
			info.HasArtwork = info.HasArtwork || hasArt
		}
	}
	for _, s := range subs {
		info.SubtitleTracks = append(info.SubtitleTracks, SubtitleTrack{
			Index:    s.Index,
			Codec:    s.Codec,
			Language: s.Language,
			Title:    s.Title,
			Embedded: true,
		})
	}
	return info, nil
}

func probeOther(r io.ReaderAt, size int64, id format.ID, info *ProbeInfo) (*ProbeInfo, error) {
	switch id {
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
	case format.AVI:
		info.FormatName = "avi"
	case format.ASF, format.WMV:
		info.FormatName = "asf"
	case format.MPEGTS:
		info.FormatName = "mpegts"
	case format.M2TS:
		info.FormatName = "m2ts"
	case format.MPEGPS:
		info.FormatName = "mpeg"
	case format.HEIC:
		info.FormatName = "heic"
	case format.HEIF:
		info.FormatName = "heif"
	case format.AVIF:
		info.FormatName = "avif"
	case format.CR2, format.CR3, format.NEF, format.ARW, format.DNG, format.ORF, format.RW2, format.RAF, format.PEF:
		info.FormatName = string(id)
	default:
		info.FormatName = string(id)
	}

	if format.IsHEIFImage(id) || format.IsCameraRAW(id) {
		return info, nil
	}

	if dr, err := Duration(r, size); err == nil {
		info.Duration = dr.Duration
		info.DurationSrc = dr.Source
	}

	// Matroska/WebM: parse the header once. The stream list and the subtitle
	// list are both derived from the same parse (the subtitle filter matches
	// ListSubtitleTracks), instead of re-reading the header for each.
	isMKV := id == format.MKV || id == format.WebM
	var mkvStreams []detect.Stream
	if isMKV {
		if raw, err := matroska.Streams(r, size); err == nil {
			mkvStreams = raw
			info.Streams = mapStreams(raw)
		}
	} else if streams, err := streamsForFormat(r, size, id); err == nil {
		info.Streams = streams
	}

	// One metadata read yields tags, lyrics presence and artwork presence.
	// (Previously ReadTags, HasEmbeddedLyrics and HasArtwork each re-read it.)
	tags, hasArt, hasLyrics, tagErr := tagread.InspectTags(r, size, id)
	if tagErr == nil {
		info.Tags = mapTags(tags)
	}
	info.HasLyrics = tagErr == nil && hasLyrics
	if id == format.MP4 {
		info.HasArtwork = isobmff.HasCover(r, size)
	} else {
		info.HasArtwork = tagErr == nil && hasArt
	}

	if isMKV {
		info.SubtitleTracks = subtitleTracksFromStreams(mkvStreams)
	} else {
		subs, _ := ListSubtitleTracks(r, size)
		info.SubtitleTracks = subs
	}

	return info, nil
}

// subtitleTracksFromStreams selects the subtitle streams from a parsed stream
// list. It produces the same tracks as ListSubtitleTracks for Matroska/WebM.
func subtitleTracksFromStreams(streams []detect.Stream) []SubtitleTrack {
	var out []SubtitleTrack
	for _, s := range streams {
		if s.Type != detect.StreamSubtitle {
			continue
		}
		out = append(out, SubtitleTrack{
			Index:    s.Index,
			Codec:    s.Codec,
			Language: s.Language,
			Title:    s.Title,
			Embedded: true,
		})
	}
	return out
}

// streamsForFormat reads stream metadata for a detected container.
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
	case format.MP3:
		s, err := mp3.Streams(r, size)
		return mapStreams(s), mapErr(err)
	case format.AVI:
		s, err := avi.Streams(r, size)
		return mapStreams(s), mapErr(err)
	case format.ASF, format.WMV:
		s, err := asf.Streams(r, size)
		return mapStreams(s), mapErr(err)
	case format.MPEGTS, format.M2TS:
		meta, err := ts.Probe(r, size)
		if err != nil {
			return nil, mapErr(err)
		}
		return mapStreams(ts.Streams(meta)), nil
	case format.MPEGPS:
		meta, err := ps.Probe(r, size)
		if err != nil {
			return nil, mapErr(err)
		}
		return mapStreams(ps.Streams(meta)), nil
	default:
		return nil, Unsupported("streams", string(id))
	}
}

// mapTags converts internal tag metadata to media.Tags.
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
