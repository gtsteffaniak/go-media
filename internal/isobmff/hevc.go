package isobmff

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/hevcdecode"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

// HEVCMP4Thumb caches moov index and hvcC for repeated HEVC thumbnails.
type HEVCMP4Thumb struct {
	R     io.ReaderAt
	Size  int64
	Track int
	hvcC  []byte
	index *movieIndex
}

// hevc returns the first HEVC track handle.
func (s ThumbSource) hevc() (t HEVCMP4Thumb, found bool, err error) {
	track, ok := s.idx.firstHEVC()
	if !ok {
		return HEVCMP4Thumb{}, false, nil
	}
	tr, err := s.idx.track(track)
	if err != nil {
		return HEVCMP4Thumb{}, true, thumbErr(err)
	}
	if len(tr.hvcC) == 0 {
		return HEVCMP4Thumb{}, true, errs.Unsup("thumbnail", "mp4: hvcC box missing from "+string(tr.entry[:])+" sample entry")
	}
	return HEVCMP4Thumb{R: s.idx.r, Size: s.idx.size, Track: track, hvcC: tr.hvcC, index: s.idx}, true, nil
}

// HEVCThumbnailYCbCr decodes the IRAP/IDR keyframe at seekPercent.
func (s ThumbSource) HEVCThumbnailYCbCr(seekPercent float64) (*h264.ThumbYCbCr, error) {
	t, found, err := s.hevc()
	if !found || err != nil {
		return nil, err
	}
	sample, err := s.idx.hevcSyncSampleErr(t.Track, seekPercent)
	if err != nil {
		return nil, thumbErr(err)
	}
	pic, err := hevcdecode.DecodeHVCCSample(t.hvcC, sample)
	if err != nil {
		return nil, err
	}
	defer pic.Release()
	ycc := hevcdecode.PictureThumbYCbCr(pic)
	if ycc == nil {
		return nil, errs.UnsupProfile("hevc keyframe", "decoder returned no picture")
	}
	return ycc, nil
}

func (m *movieIndex) hevcSyncSample(trackIndex int, seekPercent float64) ([]byte, bool) {
	b, err := m.hevcSyncSampleErr(trackIndex, seekPercent)
	return b, err == nil
}

func (m *movieIndex) hevcSyncSampleErr(trackIndex int, seekPercent float64) ([]byte, error) {
	return m.h264SyncSampleErr(trackIndex, seekPercent)
}

func isHEVCEntry(t string) bool {
	switch t {
	case "hvc1", "hev1", "dvh1", "dvhe":
		return true
	default:
		return false
	}
}

func (m *movieIndex) firstHEVC() (int, bool) {
	for i := range m.tracks {
		if isHEVCEntry(string(m.tracks[i].entry[:])) {
			return i, true
		}
	}
	return 0, false
}
