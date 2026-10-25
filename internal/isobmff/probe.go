// Package isobmff supports go-media parsers.

package isobmff

import (
	"io"
	"time"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// Duration reads container length from headers in internal.
func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return detect.DurationResult{}, err
	}
	if info.Timescale == 0 || info.Duration == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "mp4 missing mvhd duration")
	}
	sec := float64(info.Duration) / float64(info.Timescale)
	if sec < 0 || sec > 86400*366*100 {
		return detect.DurationResult{}, errs.Unsup("duration", "mp4 duration out of range")
	}
	d := time.Duration(sec * float64(time.Second))
	return detect.DurationResult{Duration: d, Source: "header"}, nil
}

// Streams returns elementary stream descriptors from container headers.
func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, err
	}
	var streams []detect.Stream
	for i, tr := range info.Tracks {
		s, ok := streamForTrack(r, size, i, tr)
		if !ok {
			continue
		}
		streams = append(streams, s)
	}
	return streams, nil
}

func streamForTrack(r io.ReaderAt, size int64, index int, tr *mp4.Track) (detect.Stream, bool) {
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
		vse, ok := visualMJPEGSampleEntry(r, size, index)
		if !ok {
			return detect.Stream{}, false
		}
		s.Type = detect.StreamVideo
		s.Codec = "mjpeg"
		s.Width = int(vse.Width)
		s.Height = int(vse.Height)
		return s, true
	default:
		return detect.Stream{}, false
	}
}

// FormatName supports media inspection.
func FormatName(r io.ReaderAt, size int64) string {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return "mp4"
	}
	brand := string(info.MajorBrand[:])
	return "mov,mp4,m4a,3gp,3g2,mj2," + brand
}
