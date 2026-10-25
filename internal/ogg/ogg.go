// Package ogg supports go-media parsers.

package ogg

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// Duration reads container length from headers in internal.
func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	head, _ := ioat.ReadAll(r, size, 0, min64(size, 128*1024))
	sr := sampleRateFromHead(head)
	if sr == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "ogg sample rate")
	}
	readLen := size
	if readLen > 512*1024 {
		readLen = 512 * 1024
	}
	tail, err := ioat.ReadAll(r, size, size-readLen, readLen)
	if err != nil && err != io.EOF {
		return detect.DurationResult{}, err
	}
	granule, ok := maxGranule(tail)
	if !ok {
		return detect.DurationResult{}, errs.Unsup("duration", "ogg granule")
	}
	sec := float64(granule) / float64(sr)
	return detect.DurationResult{Duration: time.Duration(sec * float64(time.Second)), Source: "header"}, nil
}

// sampleRateFromHead parses Vorbis or Opus sample rate from file prefix.
func sampleRateFromHead(buf []byte) int {
	if sr := vorbisSampleRate(buf); sr > 0 {
		return sr
	}
	if sr := opusSampleRate(buf); sr > 0 {
		return sr
	}
	return 0
}

// vorbisSampleRate reads sample rate from the first Vorbis identification header.
func vorbisSampleRate(buf []byte) int {
	limit := len(buf)
	if limit > 64*1024 {
		limit = 64 * 1024
	}
	for i := 0; i+16 <= limit; i++ {
		if buf[i] != 0x01 || buf[i+1] != 'v' || buf[i+2] != 'o' || buf[i+3] != 'r' || buf[i+4] != 'b' || buf[i+5] != 'i' || buf[i+6] != 's' {
			continue
		}
		sr := int(binary.LittleEndian.Uint32(buf[i+12 : i+16]))
		if sr >= 8000 && sr <= 384000 {
			return sr
		}
	}
	return 0
}

// opusSampleRate reads sample rate from an OpusHead packet.
func opusSampleRate(buf []byte) int {
	i := 0
	for i+8 < len(buf) {
		if buf[i] == 'O' && buf[i+1] == 'p' && buf[i+2] == 'u' && buf[i+3] == 's' && buf[i+4] == 'H' && buf[i+5] == 'e' && buf[i+6] == 'a' && buf[i+7] == 'd' {
			if i+16 <= len(buf) {
				return int(binary.LittleEndian.Uint32(buf[i+12 : i+16]))
			}
			return 48000
		}
		i++
	}
	return 0
}

// maxGranule finds the maximum Ogg page granule position in buf.
func maxGranule(buf []byte) (uint64, bool) {
	var maxG uint64
	found := false
	for i := 0; i+27 <= len(buf); {
		if buf[i] != 'O' || buf[i+1] != 'g' || buf[i+2] != 'g' || buf[i+3] != 'S' {
			i++
			continue
		}
		granule := uint64(buf[i+6]) | uint64(buf[i+7])<<8 | uint64(buf[i+8])<<16 | uint64(buf[i+9])<<24 |
			uint64(buf[i+10])<<32 | uint64(buf[i+11])<<40 | uint64(buf[i+12])<<48 | uint64(buf[i+13])<<56
		if granule != ^uint64(0) && granule > maxG {
			maxG = granule
			found = true
		}
		pageLen := oggPageLength(buf, i)
		if pageLen <= 0 {
			break
		}
		i += pageLen
	}
	return maxG, found
}

func oggPageLength(buf []byte, i int) int {
	if i+27 > len(buf) {
		return 0
	}
	segs := int(buf[i+26])
	if i+27+segs > len(buf) {
		return 0
	}
	total := 27 + segs
	for j := 0; j < segs; j++ {
		total += int(buf[i+27+j])
	}
	return total
}

// Streams returns elementary stream descriptors from container headers.
func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	head, _ := ioat.ReadAll(r, size, 0, min64(size, 128*1024))
	sr := sampleRateFromHead(head)
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

// contains reports whether sub appears in b.
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

// min64 returns the smaller of two int64 values.
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
