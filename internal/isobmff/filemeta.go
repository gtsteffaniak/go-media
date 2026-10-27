package isobmff

import (
	"io"
	"strings"
	"time"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// FileMeta holds one mp4.Probe plus per-trak handler/stsd metadata for a file.
type FileMeta struct {
	probe *mp4.ProbeInfo
	traks []trakMeta
}

// OpenFileMeta parses moov headers + per-trak stsd/hdlr (no stsz/stts sample tables).
func OpenFileMeta(r io.ReaderAt, size int64) (FileMeta, error) {
	h, err := readMovieHeader(r, size)
	if err != nil {
		return FileMeta{}, err
	}
	metas, err := listTrakMeta(r, size)
	if err != nil {
		return FileMeta{}, err
	}
	return FileMeta{probe: probeInfoFromHeader(h, metas), traks: metas}, nil
}

// FormatName returns an ffprobe-like format_name string.
func (m FileMeta) FormatName() string {
	if m.probe == nil {
		return "mp4"
	}
	brand := string(m.probe.MajorBrand[:])
	return "mov,mp4,m4a,3gp,3g2,mj2," + brand
}

// Duration reads mvhd duration from the cached probe.
func (m FileMeta) Duration() (detect.DurationResult, error) {
	return durationFromProbe(m.probe)
}

// durationFromProbe reads mvhd duration (no trak walk).
func durationFromProbe(info *mp4.ProbeInfo) (detect.DurationResult, error) {
	if info == nil || info.Timescale == 0 || info.Duration == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "mp4 missing mvhd duration")
	}
	sec := float64(info.Duration) / float64(info.Timescale)
	if sec < 0 || sec > 86400*366*100 {
		return detect.DurationResult{}, errs.Unsup("duration", "mp4 duration out of range")
	}
	d := time.Duration(sec * float64(time.Second))
	return detect.DurationResult{Duration: d, Source: "header"}, nil
}

// Streams returns elementary stream descriptors using cached probe + trak meta.
func (m FileMeta) Streams() []detect.Stream {
	if m.probe == nil {
		return nil
	}
	var streams []detect.Stream
	for i, tr := range m.probe.Tracks {
		meta := trakMeta{}
		if i < len(m.traks) {
			meta = m.traks[i]
		}
		if isSubtitleHandler(meta.HandlerType) {
			codec := subtitleCodecFromFourCC(meta.SampleFourCC)
			if codec != "" {
				streams = append(streams, detect.Stream{
					Index: i, Type: detect.StreamSubtitle, Codec: codec,
					Language: meta.Language, Title: meta.HandlerName,
				})
			}
			continue
		}
		s, ok := streamForTrackMeta(i, tr, meta)
		if !ok {
			continue
		}
		streams = append(streams, s)
	}
	return streams
}

// HasAttachedCover is true for ilst-style cover tracks (not full-motion MJPEG video).
func (m FileMeta) HasAttachedCover() bool {
	for _, meta := range m.traks {
		if meta.AttachedCover {
			return true
		}
	}
	return false
}

// SubtitleStreams lists embedded subtitle tracks from cached trak meta.
func (m FileMeta) SubtitleStreams() []detect.Stream {
	var out []detect.Stream
	for i, meta := range m.traks {
		if !isSubtitleHandler(meta.HandlerType) {
			continue
		}
		codec := subtitleCodecFromFourCC(meta.SampleFourCC)
		if codec == "" {
			continue
		}
		out = append(out, detect.Stream{
			Index:    i,
			Type:     detect.StreamSubtitle,
			Codec:    codec,
			Language: meta.Language,
			Title:    meta.HandlerName,
		})
	}
	return out
}

func streamForTrackMeta(index int, tr *mp4.Track, meta trakMeta) (detect.Stream, bool) {
	s := detect.Stream{Index: index}
	switch tr.Codec {
	case mp4.CodecAVC1:
		s.Type = detect.StreamVideo
		s.Codec = "h264"
		if tr.AVC != nil {
			s.Width = int(tr.AVC.Width)
			s.Height = int(tr.AVC.Height)
		}
		return s, true
	case mp4.CodecMP4A:
		s.Type = detect.StreamAudio
		s.Codec = "aac"
		if tr.MP4A != nil {
			s.Channels = int(tr.MP4A.ChannelCount)
		}
		return s, true
	case mp4.CodecUnknown:
		if !isMotionMJPEGTrak(meta) {
			return detect.Stream{}, false
		}
		s.Type = detect.StreamVideo
		s.Codec = "mjpeg"
		s.Width = meta.Width
		s.Height = meta.Height
		return s, true
	default:
		return detect.Stream{}, false
	}
}

func isMotionMJPEGTrak(meta trakMeta) bool {
	if meta.AttachedCover {
		return false
	}
	if meta.HandlerType != "vide" {
		return false
	}
	switch strings.TrimSpace(meta.SampleFourCC) {
	case "mp4v", "jpeg", "jpg ":
		return true
	default:
		return false
	}
}
