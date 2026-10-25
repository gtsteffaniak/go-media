package ogg

import (
	"io"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	tailSize := min64(size, 128*1024)
	tail, err := ioat.ReadAll(r, size, size-tailSize, tailSize)
	if err != nil && err != io.EOF {
		return detect.DurationResult{}, err
	}
	granule, sr, ok := lastGranule(tail)
	if !ok || sr == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "ogg granule/sample rate")
	}
	sec := float64(granule) / float64(sr)
	return detect.DurationResult{Duration: time.Duration(sec * float64(time.Second)), Source: "header"}, nil
}

func lastGranule(buf []byte) (uint64, int, bool) {
	var lastGranule uint64
	var sampleRate int
	for i := 0; i+27 <= len(buf); i++ {
		if buf[i] != 'O' || buf[i+1] != 'g' || buf[i+2] != 'g' || buf[i+3] != 'S' {
			continue
		}
		granule := uint64(buf[i+6]) | uint64(buf[i+7])<<8 | uint64(buf[i+8])<<16 | uint64(buf[i+9])<<24 |
			uint64(buf[i+10])<<32 | uint64(buf[i+11])<<40 | uint64(buf[i+12])<<48 | uint64(buf[i+13])<<56
		if granule != 0 {
			lastGranule = granule
		}
		if sampleRate == 0 {
			sampleRate = guessSampleRate(buf[i:])
		}
	}
	if lastGranule == 0 {
		return 0, 0, false
	}
	if sampleRate == 0 {
		sampleRate = 48000
	}
	return lastGranule, sampleRate, true
}

func guessSampleRate(page []byte) int {
	for _, sr := range []int{8000, 11025, 16000, 22050, 32000, 44100, 48000, 96000} {
		b := byte(sr & 0xFF)
		b1 := byte((sr >> 8) & 0xFF)
		b2 := byte((sr >> 16) & 0xFF)
		b3 := byte((sr >> 24) & 0xFF)
		for i := 0; i+4 < len(page); i++ {
			if page[i] == b3 && page[i+1] == b2 && page[i+2] == b1 && page[i+3] == b {
				return sr
			}
		}
	}
	return 48000
}

func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	head, _ := ioat.ReadAll(r, size, 0, min64(size, 4096))
	_, sr, _ := lastGranule(head)
	if sr == 0 {
		sr = 48000
	}
	codec := "vorbis"
	if contains(head, []byte("OpusHead")) {
		codec = "opus"
	}
	return []detect.Stream{{
		Index:      0,
		Type:       detect.StreamAudio,
		Codec:      codec,
		SampleRate: sr,
		Channels:   2,
	}}, nil
}

func contains(b, sub []byte) bool {
	for i := 0; i+len(sub) <= len(b); i++ {
		ok := true
		for j := range sub {
			if b[i+j] != sub[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
