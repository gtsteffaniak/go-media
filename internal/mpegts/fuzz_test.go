package ts_test

import (
	"testing"

	ts "github.com/gtsteffaniak/go-media/internal/mpegts"
)

func FuzzProbe(f *testing.F) {
	f.Add([]byte{0x47, 0x40, 0x00, 0x10})
	f.Fuzz(func(t *testing.T, b []byte) {
		r := bytesReaderAt(b)
		_, _ = ts.Probe(r, int64(len(b)))
	})
}

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, nil
	}
	return copy(p, b[off:]), nil
}
