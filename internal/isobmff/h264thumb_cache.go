package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

// H264MP4Thumb caches one moov index and avcC per file for repeated thumbnails (same ReaderAt).
type H264MP4Thumb struct {
	R     io.ReaderAt
	Size  int64
	Track int
	AVCC  *mp4.AVCDecoderConfiguration
	index *movieIndex
}

// OpenH264MP4Thumb indexes moov once and reads avcC for the first H.264 track.
func OpenH264MP4Thumb(r io.ReaderAt, size int64) (H264MP4Thumb, bool) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return H264MP4Thumb{}, false
	}
	track, ok := idx.firstAVC()
	if !ok {
		return H264MP4Thumb{}, false
	}
	tr, err := idx.track(track)
	if err != nil {
		return H264MP4Thumb{}, false
	}
	avcc, ok := tr.avcConfig()
	if !ok {
		return H264MP4Thumb{}, false
	}
	return H264MP4Thumb{R: r, Size: size, Track: track, AVCC: avcc, index: idx}, true
}

// syncSample returns the keyframe payload at seekPercent using the cached index.
func (t H264MP4Thumb) syncSample(seekPercent float64) ([]byte, bool) {
	if t.index == nil {
		return H264SyncSample(t.R, t.Size, t.Track, seekPercent)
	}
	return t.index.h264SyncSample(t.Track, seekPercent)
}

// ThumbnailYCbCr decodes an IDR at seekPercent using cached index state.
func (t H264MP4Thumb) ThumbnailYCbCr(seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	sample, ok := t.syncSample(seekPercent)
	if !ok {
		return nil, nil
	}
	fr, err := h264.DecodeAVCCFrame(t.AVCC, sample, fast)
	if err != nil {
		return nil, err
	}
	if fr == nil {
		return nil, nil
	}
	return h264.NewThumbYCbCr(fr), nil
}
