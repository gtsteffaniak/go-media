// Package wav supports go-media parsers.

package wav

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// Duration reads container length from headers in internal.
func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	if size < 12 {
		return detect.DurationResult{}, errs.Corrupt
	}
	// One scratch buffer serves the RIFF header, chunk headers and fmt body.
	// It is only passed to ReadAt, so a single allocation covers all reads.
	buf := make([]byte, 16)
	if _, err := r.ReadAt(buf[:12], 0); err != nil {
		return detect.DurationResult{}, err
	}
	if string(buf[0:4]) != "RIFF" || string(buf[8:12]) != "WAVE" {
		return detect.DurationResult{}, errs.NotMedia
	}
	var byteRate uint32
	var dataSize int64
	pos := int64(12)
	for pos+8 <= size {
		ch := buf[:8]
		if _, err := r.ReadAt(ch, pos); err != nil {
			break
		}
		isData := string(ch[0:4]) == "data"
		sz := int64(binary.LittleEndian.Uint32(ch[4:8]))
		switch {
		case string(ch[0:4]) == "fmt ":
			if sz >= 16 && pos+8+16 <= size {
				fmtb := buf[:16]
				_, _ = r.ReadAt(fmtb, pos+8)
				byteRate = binary.LittleEndian.Uint32(fmtb[8:12])
			}
		case isData:
			dataSize = sz
		}
		if isData {
			break
		}
		pos += 8 + sz
		if sz%2 == 1 {
			pos++
		}
	}
	if byteRate == 0 || dataSize == 0 {
		return detect.DurationResult{}, errs.Unsup("duration", "wav missing fmt/data")
	}
	sec := float64(dataSize) / float64(byteRate)
	return detect.DurationResult{Duration: time.Duration(sec * float64(time.Second)), Source: "header"}, nil
}

// Streams returns elementary stream descriptors from container headers.
func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	if size < 12 {
		return nil, errs.Corrupt
	}
	var channels uint16
	var sampleRate uint32
	buf := make([]byte, 16)
	pos := int64(12)
	for pos+8 <= size {
		ch := buf[:8]
		if _, err := r.ReadAt(ch, pos); err != nil {
			break
		}
		sz := int64(binary.LittleEndian.Uint32(ch[4:8]))
		if string(ch[0:4]) == "fmt " && sz >= 16 && pos+8+16 <= size {
			fmtb := buf[:16]
			_, _ = r.ReadAt(fmtb, pos+8)
			channels = binary.LittleEndian.Uint16(fmtb[2:4])
			sampleRate = binary.LittleEndian.Uint32(fmtb[4:8])
			break
		}
		pos += 8 + sz
		if sz%2 == 1 {
			pos++
		}
	}
	return []detect.Stream{{
		Index:      0,
		Type:       detect.StreamAudio,
		Codec:      "pcm_s16le",
		SampleRate: int(sampleRate),
		Channels:   int(channels),
	}}, nil
}

var _ = io.EOF
