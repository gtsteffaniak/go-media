package matroska

import (
	"io"
	"time"

	"github.com/at-wat/ebml-go"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

type segmentHead struct {
	Segment struct {
		Info struct {
			TimecodeScale uint64  `ebml:"TimecodeScale"`
			Duration      float64 `ebml:"Duration"`
		} `ebml:"Info,stop"`
		Tracks struct {
			TrackEntry []trackEntry `ebml:"TrackEntry"`
		} `ebml:"Tracks,stop"`
	}
}

type trackEntry struct {
	TrackNumber uint64 `ebml:"TrackNumber"`
	TrackType   uint64 `ebml:"TrackType"`
	CodecID     string `ebml:"CodecID"`
	Language    string `ebml:"Language"`
	Name        string `ebml:"Name"`
	Video       *struct {
		PixelWidth  uint64 `ebml:"PixelWidth"`
		PixelHeight uint64 `ebml:"PixelHeight"`
	} `ebml:"Video"`
	Audio *struct {
		SamplingFrequency float64 `ebml:"SamplingFrequency"`
		Channels          uint64  `ebml:"Channels"`
	} `ebml:"Audio"`
}

func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	head, err := readHead(r, size)
	if err != nil {
		return detect.DurationResult{}, err
	}
	if head.Segment.Info.Duration <= 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "mkv missing duration")
	}
	scale := head.Segment.Info.TimecodeScale
	if scale == 0 {
		scale = 1_000_000
	}
	sec := head.Segment.Info.Duration * float64(scale) / 1e9
	d := time.Duration(sec * float64(time.Second))
	return detect.DurationResult{Duration: d, Source: "header"}, nil
}

func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	head, err := readHead(r, size)
	if err != nil {
		return nil, err
	}
	var out []detect.Stream
	for i, t := range head.Segment.Tracks.TrackEntry {
		s := detect.Stream{Index: i}
		switch t.TrackType {
		case 1:
			s.Type = detect.StreamVideo
			if t.Video != nil {
				s.Width = int(t.Video.PixelWidth)
				s.Height = int(t.Video.PixelHeight)
			}
			s.Codec = normalizeCodec(t.CodecID)
		case 2:
			s.Type = detect.StreamAudio
			if t.Audio != nil {
				s.SampleRate = int(t.Audio.SamplingFrequency)
				s.Channels = int(t.Audio.Channels)
			}
			s.Codec = normalizeCodec(t.CodecID)
		case 17:
			s.Type = detect.StreamSubtitle
			s.Codec = normalizeCodec(t.CodecID)
		default:
			continue
		}
		s.Language = t.Language
		s.Title = t.Name
		out = append(out, s)
	}
	return out, nil
}

func readHead(r io.ReaderAt, size int64) (*segmentHead, error) {
	reader := io.NewSectionReader(ioat.New(r, size), 0, size)
	var head segmentHead
	if err := ebml.Unmarshal(reader, &head); err != nil && err != ebml.ErrReadStopped {
		return nil, err
	}
	return &head, nil
}

func normalizeCodec(id string) string {
	switch id {
	case "V_VP8":
		return "vp8"
	case "V_VP9":
		return "vp9"
	case "V_MPEG4/ISO/AVC":
		return "h264"
	case "A_VORBIS":
		return "vorbis"
	case "A_OPUS":
		return "opus"
	default:
		return id
	}
}
