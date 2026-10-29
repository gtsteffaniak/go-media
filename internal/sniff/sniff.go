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
	// The DocType ("webm" or "matroska") is in the EBML header element at the file
	// start. Read only that element. An unusable or oversized header is treated as
	// Matroska without a further scan, which keeps sniffing to a few bytes. The
	// thumbnail path then decides from the bounded track header.
	hdr, ok := ebmlHeaderBytes(r, size)
	if !ok {
		return format.MKV, nil
	}
	if bytes.Contains(hdr, []byte("webm")) {
		return format.WebM, nil
	}
	return format.MKV, nil
}

// ebmlHeaderBytes returns the EBML header element at the start of the file (ID
// 0x1A45DFA3 plus its size and body). It reports false when the magic does not
// match, the header is not usable, or the header is larger than 4 KB.
func ebmlHeaderBytes(r io.ReaderAt, size int64) ([]byte, bool) {
	probe := make([]byte, 12)
	n, _ := ioat.ReadRange(r, size, 0, probe)
	if n < 5 || !bytes.Equal(probe[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return nil, false
	}
	// Size vint starts at offset 4. Its length is the position of the first set bit.
	first := probe[4]
	sizeLen := 0
	for i := 1; i <= 8; i++ {
		if first&(0x80>>(i-1)) != 0 {
			sizeLen = i
			break
		}
	}
	if sizeLen == 0 || 4+sizeLen > n {
		return nil, false
	}
	dataSize := uint64(first) & (0xFF >> sizeLen)
	for _, b := range probe[5 : 4+sizeLen] {
		dataSize = dataSize<<8 | uint64(b)
	}
	total := 4 + int64(sizeLen) + int64(dataSize)
	if total <= 0 || total > 4096 || total > size {
		return nil, false
	}
	buf := make([]byte, total)
	m, _ := ioat.ReadRange(r, size, 0, buf)
	if int64(m) != total {
		return nil, false
	}
	return buf, true
}
