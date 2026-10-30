package isobmff

import (
	"encoding/binary"
	"errors"
	"io"
	"strings"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// errStopWalk ends a walkBoxes loop early.
var errStopWalk = errors.New("stop")

type movieHeader struct {
	MajorBrand [4]byte
	Timescale  uint32
	Duration   uint64
}

// readMovieHeader reads ftyp and moov/mvhd by hopping top-level box headers,
// stopping at moov instead of walking every box in the file.
func readMovieHeader(r io.ReaderAt, size int64) (movieHeader, error) {
	var h movieHeader
	var hdr [16]byte
	var off int64
	for i := 0; off+8 <= size && i < maxTopLevelBoxes; i++ {
		typ, hl, bs, err := readBoxHeader(r, size, off, size, hdr[:])
		if err != nil {
			break
		}
		switch typ {
		case [4]byte{'f', 't', 'y', 'p'}:
			if b, err := readPayload(r, off+hl, min(off+bs, off+hl+4), 4); err == nil && len(b) == 4 {
				copy(h.MajorBrand[:], b)
			}
		case [4]byte{'m', 'o', 'o', 'v'}:
			_ = walkBoxes(r, size, off+hl, off+bs, func(typ [4]byte, ps, pe int64) error {
				if typ != [4]byte{'m', 'v', 'h', 'd'} {
					return nil
				}
				b, err := readPayload(r, ps, min(pe, ps+32), 32)
				if err != nil || len(b) < 20 {
					return nil
				}
				if b[0] == 1 {
					if len(b) >= 32 {
						h.Timescale = binary.BigEndian.Uint32(b[20:24])
						h.Duration = binary.BigEndian.Uint64(b[24:32])
					}
				} else {
					h.Timescale = binary.BigEndian.Uint32(b[12:16])
					h.Duration = uint64(binary.BigEndian.Uint32(b[16:20]))
				}
				return errStopWalk
			})
			i = maxTopLevelBoxes // moov found: stop
		}
		off += bs
	}
	if h.Timescale == 0 {
		return movieHeader{}, errs.Unsup("duration", "mp4 missing mvhd")
	}
	if h.Duration == 0 {
		// Fragmented MP4: mvhd carries no length, so derive it from mehd or moof.
		// Track IDs and timescales come from the lazy moov index (headers only).
		if idx, err := openMovieIndex(r, size); err == nil {
			ts := make(map[uint32]uint32, len(idx.tracks))
			for _, t := range idx.tracks {
				if t.trackID != 0 && t.timescale != 0 {
					ts[t.trackID] = t.timescale
				}
			}
			if sec, ok := fragmentDurationSeconds(r, size, h.Timescale, ts); ok {
				h.Duration = uint64(sec * float64(h.Timescale))
			}
		}
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
