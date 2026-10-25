// Package mp3 supports go-media parsers.

package mp3

import (
	"io"
	"time"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// Duration reads container length from headers in internal.
func Duration(r io.ReaderAt, size int64) (detect.DurationResult, error) {
	if size < 128 {
		return detect.DurationResult{}, errs.Corrupt
	}
	audioStart := int64(0)
	head, _ := ioat.ReadAll(r, size, 0, min64(size, 12))
	if len(head) >= 3 && head[0] == 'I' && head[1] == 'D' && head[2] == '3' {
		id3h, _ := ioat.ReadAll(r, size, 0, 10)
		if len(id3h) < 10 {
			return detect.DurationResult{}, errs.Corrupt
		}
		tagSize := syncSafe(id3h[6:10])
		audioStart = 10 + int64(tagSize)
	}
	if audioStart >= size {
		return detect.DurationResult{}, errs.Unsup("duration", "mp3 audio start past eof")
	}
	frameBuf, _ := ioat.ReadAll(r, size, audioStart, min64(size-audioStart, 256*1024))
	if d, src, ok := durationFromXingScan(frameBuf); ok {
		return detect.DurationResult{Duration: d, Source: src}, nil
	}
	d, src, ok := durationFromFrames(frameBuf, 0, size-audioStart)
	if !ok {
		return detect.DurationResult{}, errs.Unsup("duration", "mp3 without xing/vbri and cbr estimate failed")
	}
	return detect.DurationResult{Duration: d, Source: src}, nil
}

// syncSafe decodes an ID3v2 synchsafe integer.
func syncSafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

var bitrates = [16]int{
	0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0,
}

var sampleRates = [4]int{44100, 48000, 32000, 0}

// durationFromFrames estimates MP3 length from frame headers.
func durationFromFrames(buf []byte, start int, fileSize int64) (time.Duration, string, bool) {
	pos := start
	for pos+4 < len(buf) {
		if buf[pos] != 0xFF || (buf[pos+1]&0xE0) != 0xE0 {
			pos++
			continue
		}
		hdr := uint32(buf[pos])<<24 | uint32(buf[pos+1])<<16 | uint32(buf[pos+2])<<8 | uint32(buf[pos+3])
		version := (hdr >> 19) & 3
		layer := (hdr >> 17) & 3
		brIdx := int((hdr >> 12) & 0xF)
		srIdx := int((hdr >> 10) & 3)
		padding := int((hdr >> 9) & 1)
		if layer != 1 || version == 1 {
			pos++
			continue
		}
		br := bitrates[brIdx]
		sr := sampleRates[srIdx]
		if br == 0 || sr == 0 {
			pos++
			continue
		}
		frameLen := 144*br*1000/sr + padding
		if frameLen < 4 {
			pos++
			continue
		}
		side := mpegSideInfoBytes(version, buf[pos+3])
		tagOff := pos + 4 + side
		if tagOff+12 <= len(buf) {
			tag := string(buf[tagOff : tagOff+4])
			if tag == "Xing" || tag == "Info" {
				if d, src, ok := durationFromXing(buf[tagOff:], version, sr); ok {
					return d, src, true
				}
			}
			if tagOff+26 <= len(buf) && string(buf[tagOff:tagOff+4]) == "VBRI" {
				frames := int(uint32(buf[tagOff+14])<<24 | uint32(buf[tagOff+15])<<16 | uint32(buf[tagOff+16])<<8 | uint32(buf[tagOff+17]))
				samplesPerFrame := 1152
				if version != 3 {
					samplesPerFrame = 576
				}
				sec := float64(frames*samplesPerFrame) / float64(sr)
				return time.Duration(sec * float64(time.Second)), "header", true
			}
		}
		pos += frameLen
	}
	audioBytes := fileSize - int64(start)
	if audioBytes > 0 {
		if d, src, ok := cbrEstimate(buf, start, audioBytes); ok {
			return d, src, true
		}
	}
	return 0, "", false
}

func mpegSideInfoBytes(version uint32, b3 byte) int {
	mode := (b3 >> 6) & 0x03
	if version == 3 {
		if mode == 3 {
			return 17
		}
		return 32
	}
	return 17
}

func durationFromXingScan(buf []byte) (time.Duration, string, bool) {
	for i := 0; i+4 < len(buf); i++ {
		if buf[i] != 'X' && buf[i] != 'I' {
			continue
		}
		if i+4 > len(buf) {
			break
		}
		tag := string(buf[i : i+4])
		if tag != "Xing" && tag != "Info" {
			continue
		}
		// Walk back to frame header for version/sample rate.
		start := i - 4 - 32
		if start < 0 {
			start = 0
		}
		for pos := start; pos+4 < i; pos++ {
			if buf[pos] != 0xFF || (buf[pos+1]&0xE0) != 0xE0 {
				continue
			}
			hdr := uint32(buf[pos])<<24 | uint32(buf[pos+1])<<16 | uint32(buf[pos+2])<<8 | uint32(buf[pos+3])
			version := (hdr >> 19) & 3
			layer := (hdr >> 17) & 3
			srIdx := int((hdr >> 10) & 3)
			if layer != 1 || version == 1 || srIdx >= len(sampleRates) || sampleRates[srIdx] == 0 {
				continue
			}
			if d, src, ok := durationFromXing(buf[i:], version, sampleRates[srIdx]); ok {
				return d, src, true
			}
		}
	}
	return 0, "", false
}

func durationFromXing(xing []byte, version uint32, sr int) (time.Duration, string, bool) {
	if len(xing) < 12 {
		return 0, "", false
	}
	flags := uint32(xing[4])<<24 | uint32(xing[5])<<16 | uint32(xing[6])<<8 | uint32(xing[7])
	off := 8
	samplesPerFrame := 1152
	if version != 3 {
		samplesPerFrame = 576
	}
	if flags&0x01 != 0 && off+4 <= len(xing) {
		frames := int(uint32(xing[off])<<24 | uint32(xing[off+1])<<16 | uint32(xing[off+2])<<8 | uint32(xing[off+3]))
		off += 4
		sec := float64(frames*samplesPerFrame) / float64(sr)
		return time.Duration(sec * float64(time.Second)), "header", true
	}
	if flags&0x02 != 0 && off+4 <= len(xing) {
		audioBytes := int(uint32(xing[off])<<24 | uint32(xing[off+1])<<16 | uint32(xing[off+2])<<8 | uint32(xing[off+3]))
		off += 4
		if brIdx := firstFrameBitrateIndex(xing); brIdx > 0 {
			br := bitrates[brIdx]
			if br > 0 {
				sec := float64(audioBytes*8) / float64(br*1000)
				return time.Duration(sec * float64(time.Second)), "header", true
			}
		}
	}
	return 0, "", false
}

func firstFrameBitrateIndex(buf []byte) int {
	for pos := 0; pos+4 < len(buf); pos++ {
		if buf[pos] != 0xFF || (buf[pos+1]&0xE0) != 0xE0 {
			continue
		}
		hdr := uint32(buf[pos])<<24 | uint32(buf[pos+1])<<16 | uint32(buf[pos+2])<<8 | uint32(buf[pos+3])
		layer := (hdr >> 17) & 3
		version := (hdr >> 19) & 3
		if layer != 1 || version == 1 {
			continue
		}
		return int((hdr >> 12) & 0xF)
	}
	return 0
}

func cbrEstimate(buf []byte, start int, audioBytes int64) (time.Duration, string, bool) {
	pos := start
	for pos+4 < len(buf) {
		if buf[pos] != 0xFF || (buf[pos+1]&0xE0) != 0xE0 {
			pos++
			continue
		}
		hdr := uint32(buf[pos])<<24 | uint32(buf[pos+1])<<16 | uint32(buf[pos+2])<<8 | uint32(buf[pos+3])
		version := (hdr >> 19) & 3
		layer := (hdr >> 17) & 3
		brIdx := int((hdr >> 12) & 0xF)
		srIdx := int((hdr >> 10) & 3)
		if layer != 1 || version == 1 {
			pos++
			continue
		}
		br := bitrates[brIdx]
		sr := sampleRates[srIdx]
		if br == 0 || sr == 0 {
			pos++
			continue
		}
		sec := float64(audioBytes*8) / float64(br*1000)
		return time.Duration(sec * float64(time.Second)), "estimate", true
	}
	return 0, "", false
}

// Streams returns a single audio stream descriptor from the first MP3 frame header.
func Streams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	if size < 4 {
		return nil, errs.Corrupt
	}
	audioStart := int64(0)
	head, _ := ioat.ReadAll(r, size, 0, min64(size, 12))
	if len(head) >= 3 && head[0] == 'I' && head[1] == 'D' && head[2] == '3' {
		id3h, _ := ioat.ReadAll(r, size, 0, 10)
		if len(id3h) >= 10 {
			audioStart = 10 + int64(syncSafe(id3h[6:10]))
		}
	}
	if audioStart >= size {
		return nil, errs.Unsup("streams", "mp3 audio start past eof")
	}
	frame, _ := ioat.ReadAll(r, size, audioStart, min64(size-audioStart, 4096))
	sr, ch := firstFrameAudioParams(frame)
	if sr == 0 {
		sr = 44100
	}
	if ch == 0 {
		ch = 2
	}
	return []detect.Stream{{
		Index: 0, Type: detect.StreamAudio, Codec: "mp3",
		SampleRate: sr, Channels: ch,
	}}, nil
}

func firstFrameAudioParams(buf []byte) (sampleRate, channels int) {
	for pos := 0; pos+4 < len(buf); pos++ {
		if buf[pos] != 0xFF || (buf[pos+1]&0xE0) != 0xE0 {
			continue
		}
		hdr := uint32(buf[pos])<<24 | uint32(buf[pos+1])<<16 | uint32(buf[pos+2])<<8 | uint32(buf[pos+3])
		version := (hdr >> 19) & 3
		layer := (hdr >> 17) & 3
		srIdx := int((hdr >> 10) & 3)
		if layer != 1 || version == 1 || srIdx >= len(sampleRates) || sampleRates[srIdx] == 0 {
			continue
		}
		mode := (buf[pos+3] >> 6) & 0x03
		ch := 2
		if mode == 3 {
			ch = 1
		}
		return sampleRates[srIdx], ch
	}
	return 0, 0
}

// min64 returns the smaller of two int64 values.
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
