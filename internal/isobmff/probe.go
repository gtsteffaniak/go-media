package isobmff

import (
	"io"
	"time"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return detect.DurationResult{}, err
	}
	if info.Timescale == 0 || info.Duration == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "mp4 missing mvhd duration")
	}
	d := time.Duration(info.Duration) * time.Second / time.Duration(info.Timescale)
	return detect.DurationResult{Duration: d, Source: "header"}, nil
}

func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, err
	}
	var streams []detect.Stream
	for i, tr := range info.Tracks {
		s := detect.Stream{Index: i}
		switch tr.Codec {
		case mp4.CodecAVC1:
			s.Type = detect.StreamVideo
			s.Codec = "h264"
			if tr.AVC != nil {
				s.Width = int(tr.AVC.Width)
				s.Height = int(tr.AVC.Height)
			}
		case mp4.CodecMP4A:
			s.Type = detect.StreamAudio
			s.Codec = "aac"
			if tr.MP4A != nil {
				s.Channels = int(tr.MP4A.ChannelCount)
			}
		default:
			continue
		}
		streams = append(streams, s)
	}
	return streams, nil
}

func FormatName(r io.ReaderAt, size int64) string {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return "mp4"
	}
	brand := string(info.MajorBrand[:])
	return "mov,mp4,m4a,3gp,3g2,mj2," + brand
}
