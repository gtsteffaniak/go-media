package isobmff_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

func TestAV1MP4Thumbnail(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "s8_1080.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, _ := f.Stat()
	src, err := isobmff.OpenThumbSourceErr(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	ycc, err := src.AV1ThumbnailYCbCr(10)
	if err != nil {
		t.Fatal(err)
	}
	if ycc == nil || ycc.Bounds().Dx() < 100 {
		t.Fatalf("bad ycc %v", ycc)
	}
}
