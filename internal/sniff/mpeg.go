package sniff

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/format"
	ps "github.com/gtsteffaniak/go-media/internal/mpegps"
	ts "github.com/gtsteffaniak/go-media/internal/mpegts"
)

func sniffMPEGPS(r io.ReaderAt, size int64) bool {
	return ps.Sniff(r, size)
}

func sniffMPEGTS(r io.ReaderAt, size int64) (format.ID, bool) {
	lay, ok := ts.DetectLayout(r, size)
	if !ok {
		return "", false
	}
	if lay.M2TS {
		return format.M2TS, true
	}
	return format.MPEGTS, true
}
