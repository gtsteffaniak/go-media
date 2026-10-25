package isobmff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

func TestH264BaselineThumbnailRejected(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264_baseline_1080.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path)
	}
	defer f.Close()
	st, _ := f.Stat()
	_, err = H264ThumbnailRGBA(f, st.Size(), 10, false)
	if err == nil {
		t.Fatal("expected baseline profile to be rejected")
	}
	if !errors.Is(err, errs.UnsupportedProfile) {
		t.Fatalf("expected ErrUnsupportedProfile, got %v", err)
	}
}
