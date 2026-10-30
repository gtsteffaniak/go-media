// Package isobmff supports go-media parsers.
//
// H.264 thumbnails seek to an MP4 sync sample (stss), which may be an IDR or
// an open-GOP keyframe; see internal/h264 for which ones decode and how
// colour is chosen.
package isobmff

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/detect"
)

// Duration reads container length from mvhd only (no sample tables).
func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	h, err := readMovieHeader(r, size)
	if err != nil {
		return detect.DurationResult{}, err
	}
	return durationFromMovieHeader(h)
}

// Streams returns elementary stream descriptors from container headers.
func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	m, err := OpenFileMeta(r, size)
	if err != nil {
		return nil, err
	}
	return m.Streams(), nil
}

// FormatName supports media inspection (ftyp only).
func FormatName(r io.ReaderAt, size int64) string {
	h, err := readMovieHeader(r, size)
	if err != nil {
		return "mp4"
	}
	brand := string(h.MajorBrand[:])
	return "mov,mp4,m4a,3gp,3g2,mj2," + brand
}
