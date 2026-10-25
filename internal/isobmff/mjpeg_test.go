package isobmff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func TestMJPEGFrameData(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "frame.mjpeg.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	b, ok := MJPEGFrame(ioat.New(f, st.Size()), st.Size())
	if !ok || len(b) < 10 {
		t.Fatalf("mjpeg frame: ok=%v len=%d", ok, len(b))
	}
}
