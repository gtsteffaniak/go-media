package isobmff

import (
	"image"
	"io"

	"github.com/gtsteffaniak/go-media/internal/h264"
)

// ThumbSource indexes moov once for the thumbnail path so the rotation check,
// the MJPEG lookup and the H.264 keyframe seek share one header walk.
type ThumbSource struct {
	idx *movieIndex
}

// OpenThumbSource indexes the MP4 moov box with bounded header reads.
// ok is false when the file has no indexable moov.
func OpenThumbSource(r io.ReaderAt, size int64) (ThumbSource, bool) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return ThumbSource{}, false
	}
	return ThumbSource{idx: idx}, true
}

// Rotation returns the display rotation in degrees of the first motion-video
// track (ffprobe sign convention: -90 means rotate 90 degrees clockwise).
func (s ThumbSource) Rotation() int {
	rot, _ := s.idx.videoRotation()
	return rot
}

// MJPEGFrame returns the JPEG sample nearest seekPercent of an MJPEG track.
func (s ThumbSource) MJPEGFrame(seekPercent float64) ([]byte, bool) {
	return s.idx.mjpegFrame(seekPercent)
}

// h264 returns the cached H.264 handle for the first AVC track.
func (s ThumbSource) h264() (H264MP4Thumb, bool) {
	track, ok := s.idx.firstAVC()
	if !ok {
		return H264MP4Thumb{}, false
	}
	tr, err := s.idx.track(track)
	if err != nil {
		return H264MP4Thumb{}, false
	}
	avcc, ok := tr.avcConfig()
	if !ok {
		return H264MP4Thumb{}, false
	}
	return H264MP4Thumb{R: s.idx.r, Size: s.idx.size, Track: track, AVCC: avcc, index: s.idx}, true
}

// H264ThumbnailYCbCr decodes the IDR frame at seekPercent to YCbCr.
// It returns (nil, nil) when the file has no decodable H.264 track.
func (s ThumbSource) H264ThumbnailYCbCr(seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	t, ok := s.h264()
	if !ok {
		return nil, nil
	}
	return t.ThumbnailYCbCr(seekPercent, fast)
}

// H264ThumbnailRGBA decodes the IDR frame at seekPercent to RGBA.
// It returns (nil, nil) when the file has no decodable H.264 track.
func (s ThumbSource) H264ThumbnailRGBA(seekPercent float64, fast bool) (*image.RGBA, error) {
	t, ok := s.h264()
	if !ok {
		return nil, nil
	}
	sample, ok := t.syncSample(seekPercent)
	if !ok {
		return nil, nil
	}
	img, err := h264.DecodeAVCC(t.AVCC, sample, fast)
	if err != nil || img == nil {
		return nil, err
	}
	return img, nil
}
