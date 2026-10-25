package wav

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	head, err := ioat.ReadAll(r, size, 0, min64(size, 256*1024))
	if err != nil {
		return detect.DurationResult{}, err
	}
	if len(head) < 44 {
		return detect.DurationResult{}, errs.Corrupt
	}
	if string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return detect.DurationResult{}, errs.NotMedia
	}
	var dataSize int64
	var byteRate uint32
	pos := 12
	for pos+8 <= len(head) {
		id := string(head[pos : pos+4])
		ch := binary.LittleEndian.Uint32(head[pos+4 : pos+8])
		pos += 8
		if pos+int(ch) > len(head) {
			break
		}
		switch id {
		case "fmt ":
			if ch >= 16 {
				byteRate = binary.LittleEndian.Uint32(head[pos+8 : pos+12])
			}
		case "data":
			dataSize = int64(ch)
		}
		pos += int(ch)
	}
	if byteRate == 0 || dataSize == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "wav missing fmt/data")
	}
	sec := float64(dataSize) / float64(byteRate)
	return detect.DurationResult{Duration: time.Duration(sec * float64(time.Second)), Source: "header"}, nil
}

func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	head, _ := ioat.ReadAll(r, size, 0, min64(size, 4096))
	if len(head) < 44 {
		return nil, errs.Corrupt
	}
	var channels uint16
	var sampleRate uint32
	pos := 12
	for pos+8 <= len(head) {
		id := string(head[pos : pos+4])
		ch := binary.LittleEndian.Uint32(head[pos+4 : pos+8])
		pos += 8
		if id == "fmt " && pos+int(ch) <= len(head) && ch >= 16 {
			channels = binary.LittleEndian.Uint16(head[pos+2 : pos+4])
			sampleRate = binary.LittleEndian.Uint32(head[pos+4 : pos+8])
		}
		pos += int(ch)
	}
	return []detect.Stream{{
		Index:      0,
		Type:       detect.StreamAudio,
		Codec:      "pcm_s16le",
		SampleRate: int(sampleRate),
		Channels:   int(channels),
	}}, nil
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
