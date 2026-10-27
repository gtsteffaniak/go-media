package isobmff

import (
	"io"
	"strings"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

type movieHeader struct {
	MajorBrand [4]byte
	Timescale  uint32
	Duration   uint64
}

func readMovieHeader(r io.ReaderAt, size int64) (movieHeader, error) {
	rs := ioat.New(r, size)
	bips, err := mp4.ExtractBoxesWithPayload(rs, nil, []mp4.BoxPath{
		{mp4.BoxTypeFtyp()},
		{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()},
	})
	if err != nil {
		return movieHeader{}, err
	}
	var h movieHeader
	for _, bip := range bips {
		switch bip.Info.Type {
		case mp4.BoxTypeFtyp():
			ft := bip.Payload.(*mp4.Ftyp)
			h.MajorBrand = ft.MajorBrand
		case mp4.BoxTypeMvhd():
			mv := bip.Payload.(*mp4.Mvhd)
			h.Timescale = mv.Timescale
			if mv.GetVersion() == 0 {
				h.Duration = uint64(mv.DurationV0)
			} else {
				h.Duration = mv.DurationV1
			}
		}
	}
	if h.Timescale == 0 {
		return movieHeader{}, errs.Unsup("duration", "mp4 missing mvhd")
	}
	return h, nil
}

func durationFromMovieHeader(h movieHeader) (detect.DurationResult, error) {
	if h.Duration == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "mp4 missing mvhd duration")
	}
	info := &mp4.ProbeInfo{Timescale: h.Timescale, Duration: h.Duration}
	return durationFromProbe(info)
}

func probeInfoFromHeader(h movieHeader, metas []trakMeta) *mp4.ProbeInfo {
	info := &mp4.ProbeInfo{
		MajorBrand: h.MajorBrand,
		Timescale:  h.Timescale,
		Duration:   h.Duration,
	}
	for _, meta := range metas {
		tr := &mp4.Track{Codec: codecForTrakMeta(meta)}
		if tr.Codec == mp4.CodecAVC1 && meta.Width > 0 && meta.Height > 0 {
			tr.AVC = &mp4.AVCDecConfigInfo{
				Width:  uint16(meta.Width),
				Height: uint16(meta.Height),
			}
		}
		info.Tracks = append(info.Tracks, tr)
	}
	return info
}

func codecForTrakMeta(meta trakMeta) mp4.Codec {
	if isSubtitleHandler(meta.HandlerType) {
		return mp4.CodecUnknown
	}
	switch strings.TrimSpace(meta.SampleFourCC) {
	case "avc1", "avc3", "encv":
		return mp4.CodecAVC1
	case "mp4a", "enca":
		return mp4.CodecMP4A
	default:
		return mp4.CodecUnknown
	}
}
