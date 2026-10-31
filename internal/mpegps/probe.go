package ps

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// Meta holds program-stream timing from a bounded read.
type Meta struct {
	Duration float64
	VideoSID byte // PES stream id (typically 0xE0)
}

// Sniff reports whether the file begins with a pack start code.
func Sniff(r io.ReaderAt, size int64) bool {
	if size < 4 {
		return false
	}
	var b [4]byte
	ioat.ReadRange(r, size, 0, b[:])
	return b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0xBA
}

// Probe estimates duration from SCR in pack headers (90 kHz).
func Probe(r io.ReaderAt, size int64) (*Meta, error) {
	if !Sniff(r, size) {
		return nil, errs.NotMedia
	}
	m := &Meta{VideoSID: 0xE0}
	var maxSCR uint64
	buf := make([]byte, 65536)
	n, _ := ioat.ReadRange(r, size, 0, buf)
	data := buf[:n]
	for i := 0; i+14 <= len(data); i++ {
		if data[i] != 0 || data[i+1] != 0 || data[i+2] != 1 || data[i+3] != 0xBA {
			continue
		}
		if scr, ok := packSCR(data[i:]); ok && scr > maxSCR {
			maxSCR = scr
		}
	}
	if maxSCR > 0 {
		m.Duration = float64(maxSCR) / 90000.0
	}
	return m, nil
}

func packSCR(pack []byte) (uint64, bool) {
	if len(pack) < 14 {
		return 0, false
	}
	// MPEG-2 pack SCR: 33 bits at 90kHz (simplified parse from pack_header)
	b := pack[4:]
	if b[0]&0xC0 != 0x40 {
		return 0, false
	}
	scr := uint64(b[0]&0x38)>>3<<30 | uint64(b[0]&0x03)<<28 | uint64(b[1])<<20 | uint64(b[2]&0xF8)>>3<<15 | uint64(b[2]&0x03)<<13 | uint64(b[3])<<5 | uint64(b[4]>>3)
	return scr, true
}

// Streams returns a single video stream placeholder for PS MPG files.
func Streams(_ *Meta) []detect.Stream {
	return []detect.Stream{{Index: 0, Type: detect.StreamVideo, Codec: "mpeg2video"}}
}

// DurationSec returns probe duration in seconds.
func DurationSec(r io.ReaderAt, size int64) (float64, error) {
	m, err := Probe(r, size)
	if err != nil {
		return 0, err
	}
	return m.Duration, nil
}
