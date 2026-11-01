package isobmff

import (
	"github.com/gtsteffaniak/go-media/internal/av1decode"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

// AV1ThumbnailYCbCr decodes the sync keyframe at seekPercent on the first AV1 track.
func (s ThumbSource) AV1ThumbnailYCbCr(seekPercent float64) (*h264.ThumbYCbCr, error) {
	track, ok := s.idx.firstAV1()
	if !ok {
		return nil, nil
	}
	tr, err := s.idx.track(track)
	if err != nil {
		return nil, thumbErr(err)
	}
	sample, err := s.idx.av1SyncSampleErr(track, seekPercent)
	if err != nil {
		return nil, thumbErr(err)
	}
	ycc, err := av1decode.DecodeSampleCached(tr.av1C, sample)
	if err != nil {
		return nil, err
	}
	if ycc == nil {
		return nil, errs.UnsupProfile("av1 keyframe", "decoder returned no picture")
	}
	return ycc, nil
}

func (m *movieIndex) av1SyncSampleErr(trackIndex int, seekPercent float64) ([]byte, error) {
	return m.h264SyncSampleErr(trackIndex, seekPercent)
}

func isAV1Entry(t string) bool {
	return t == "av01"
}

func (m *movieIndex) firstAV1() (int, bool) {
	for i := range m.tracks {
		if isAV1Entry(string(m.tracks[i].entry[:])) {
			return i, true
		}
	}
	return 0, false
}
