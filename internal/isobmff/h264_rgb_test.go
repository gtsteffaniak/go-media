package isobmff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestH264BaselineThumbnailDecodes(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264_baseline_1080.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path)
	}
	defer f.Close()
	st, _ := f.Stat()
	img, err := H264ThumbnailRGBA(f, st.Size(), 10, false)
	if err != nil {
		t.Fatalf("Baseline (CAVLC) thumbnail: %v", err)
	}
	if b := img.Bounds(); b.Dx() == 0 || b.Dy() == 0 {
		t.Fatalf("empty image %v", b)
	}
}
