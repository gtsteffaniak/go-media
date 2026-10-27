package isobmff

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenH264MP4ThumbThumbnailYCbCr(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264_baseline_1080.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	tb, ok := OpenH264MP4Thumb(f, st.Size())
	if !ok {
		t.Fatal("OpenH264MP4Thumb")
	}
	if tb.Probe == nil || tb.AVCC == nil {
		t.Fatal("missing cached probe or avcC")
	}
	yc, err := tb.ThumbnailYCbCr(10, true)
	if err != nil {
		t.Fatal(err)
	}
	if yc == nil || yc.YCbCr == nil {
		t.Fatal("nil thumb")
	}
	b := yc.YCbCr.Bounds()
	if b.Dx() < 640 || b.Dy() < 360 {
		t.Fatalf("unexpected size %v", b)
	}
}
