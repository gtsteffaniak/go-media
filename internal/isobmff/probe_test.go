package isobmff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func TestStreamsMJPEGTrackSpecific(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mjpeg.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("comparison fixture missing")
	}
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	streams, err := Streams(ioat.New(f, st.Size()), st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 {
		t.Fatalf("streams: %v", streams)
	}
	if streams[0].Type != detect.StreamVideo || streams[0].Codec != "mjpeg" {
		t.Fatalf("got %+v", streams[0])
	}
	if streams[0].Width <= 0 || streams[0].Height <= 0 {
		t.Fatalf("expected dimensions, got %dx%d", streams[0].Width, streams[0].Height)
	}
}
