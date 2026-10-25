package flac

import (
	"io"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	head, err := ioat.ReadAll(r, size, 0, min64(size, 64*1024))
	if err != nil {
		return detect.DurationResult{}, err
	}
	if len(head) < 8 || string(head[0:4]) != "fLaC" {
		return detect.DurationResult{}, errs.NotMedia
	}
	pos := 4
	for pos+4 <= len(head) {
		blockHeader := head[pos]
		blockType := blockHeader & 0x7F
		last := blockHeader&0x80 != 0
		blockSize := int(head[pos+1])<<16 | int(head[pos+2])<<8 | int(head[pos+3])
		pos += 4
		if pos+blockSize > len(head) {
			break
		}
		if blockType == 0 && blockSize >= 18 {
			data := head[pos : pos+blockSize]
			sampleRate := int(data[10])<<12 | int(data[11])<<4 | int(data[12])>>4
			totalSamples := uint64(data[13]&0x0F)<<32 | uint64(data[14])<<24 | uint64(data[15])<<16 | uint64(data[16])<<8 | uint64(data[17])
			if sampleRate > 0 && totalSamples > 0 {
				sec := float64(totalSamples) / float64(sampleRate)
				return detect.DurationResult{Duration: time.Duration(sec * float64(time.Second)), Source: "header"}, nil
			}
		}
		pos += blockSize
		if last {
			break
		}
	}
	return detect.DurationResult{}, errs.Unsup("duration", "flac missing STREAMINFO")
}

func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	head, _ := ioat.ReadAll(r, size, 0, min64(size, 64*1024))
	if len(head) < 8 {
		return nil, errs.Corrupt
	}
	pos := 4
	var sampleRate int
	var channels int
	for pos+4 <= len(head) {
		blockHeader := head[pos]
		blockType := blockHeader & 0x7F
		last := blockHeader&0x80 != 0
		blockSize := int(head[pos+1])<<16 | int(head[pos+2])<<8 | int(head[pos+3])
		pos += 4
		if blockType == 0 && pos+blockSize <= len(head) && blockSize >= 18 {
			data := head[pos : pos+blockSize]
			sampleRate = int(data[10])<<12 | int(data[11])<<4 | int(data[12])>>4
			channels = int(data[12]&0x0E)>>1 + 1
		}
		pos += blockSize
		if last {
			break
		}
	}
	return []detect.Stream{{
		Index:      0,
		Type:       detect.StreamAudio,
		Codec:      "flac",
		SampleRate: sampleRate,
		Channels:   channels,
	}}, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
