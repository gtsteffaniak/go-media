package avi

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/detect"
)

// Duration reads container length from the avih header (and stream strh fallback).
func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	f, err := openIndex(r, size)
	if err != nil {
		return detect.DurationResult{}, err
	}
	return f.duration()
}

// Streams returns elementary stream descriptors from avih/strh/strf.
func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	f, err := openIndex(r, size)
	if err != nil {
		return nil, err
	}
	out := make([]detect.Stream, len(f.streams))
	for i, s := range f.streams {
		st := detect.Stream{
			Index:  i,
			Type:   s.typ,
			Codec:  s.codec,
			Width:  s.width,
			Height: s.height,
		}
		if s.rate > 0 && s.scale > 0 {
			st.FrameRate = float64(s.rate) / float64(s.scale)
		}
		out[i] = st
	}
	return out, nil
}
