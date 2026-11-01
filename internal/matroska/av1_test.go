package matroska_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/matroska"
)

func TestAV1MKVThumbnail(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "s8_4k.mkv")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, _ := f.Stat()
	hdr, err := matroska.ProbeHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	ycc, err := matroska.AV1ThumbYCbCrAtSeek(f, st.Size(), hdr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ycc == nil || ycc.Bounds().Dx() < 100 {
		t.Fatalf("bad ycc %v", ycc)
	}
}

func TestAV1WebMThumbnail(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "s8_4k.webm")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, _ := f.Stat()
	hdr, err := matroska.ProbeHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	ycc, err := matroska.AV1ThumbYCbCrAtSeek(f, st.Size(), hdr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ycc == nil || ycc.Bounds().Dx() < 100 {
		t.Fatalf("bad ycc %v", ycc)
	}
}
