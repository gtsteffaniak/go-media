package avi

import (
	"bytes"
	"os"
	"testing"
)

// FuzzParseRIFF walks arbitrary bytes through openIndex; it must not panic.
func FuzzParseRIFF(f *testing.F) {
	if b, err := os.ReadFile("testdata/mjpeg.avi"); err == nil {
		f.Add(b)
	}
	f.Add([]byte("RIFF\x24\x00\x00\x00AVI LIST\x1c\x00\x00\x00hdrlavih\x14\x00\x00\x00"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8<<20 {
			data = data[:8<<20]
		}
		_, _ = openIndex(bytes.NewReader(data), int64(len(data)))
	})
}
