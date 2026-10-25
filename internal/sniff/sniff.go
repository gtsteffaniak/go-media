// Package sniff supports go-media parsers.

package sniff

import (
	"bytes"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// Detect reads a prefix from r and returns the container format.
func Detect(r io.ReaderAt, size int64) (format.ID, error) {
	if size < 4 {
		return "", errs.NotMedia
	}
	buf := make([]byte, 16)
	n, _ := ioat.ReadRange(r, size, 0, buf)
	buf = buf[:n]
	if len(buf) < 4 {
		return "", errs.NotMedia
	}

	switch {
	case bytes.Equal(buf[:4], []byte("fLaC")):
		return format.FLAC, nil
	case bytes.Equal(buf[:4], []byte("OggS")):
		return format.OGG, nil
	case bytes.Equal(buf[:3], []byte("ID3")):
		return format.MP3, nil
	case bytes.Equal(buf[:4], []byte("RIFF")) && len(buf) >= 12 && bytes.Equal(buf[8:12], []byte("WAVE")):
		return format.WAV, nil
	case bytes.Equal(buf[:4], []byte(".snd")) || (len(buf) >= 4 && buf[0] == 0x2e && buf[1] == 's' && buf[2] == 'n' && buf[3] == 'd'):
		return format.AU, nil
	case len(buf) >= 12 && bytes.Equal(buf[4:8], []byte("ftyp")):
		return format.MP4, nil
	case bytes.Equal(buf[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return sniffEBML(r, size)
	case bytes.Equal(buf[:4], []byte("FORM")) && len(buf) >= 12 && bytes.Equal(buf[8:12], []byte("AIFF")):
		return format.AIFF, nil
	case buf[0] == 0xFF && (buf[1]&0xE0) == 0xE0:
		return format.MP3, nil
	default:
		if len(buf) >= 12 && bytes.Equal(buf[4:8], []byte("ftyp")) {
			return format.MP4, nil
		}
	}
	return "", errs.NotMedia
}

// sniffEBML distinguishes WebM from Matroska.
func sniffEBML(r io.ReaderAt, size int64) (format.ID, error) {
	chunk, err := ioat.ReadAll(r, size, 0, min64(size, 256*1024))
	if err != nil && err != io.EOF {
		return format.MKV, nil
	}
	if bytes.Contains(chunk, []byte("webm")) {
		return format.WebM, nil
	}
	return format.MKV, nil
}

// min64 returns the smaller of two int64 values.
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
