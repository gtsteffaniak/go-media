package heifthumb_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
)

func TestAVIFCodedPrimary(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "i8_1080.avif")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, _ := f.Stat()
	ycc, err := heifthumb.CodedAVIFYCbCr(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if ycc == nil || ycc.Bounds().Dx() < 100 {
		t.Fatalf("bad ycc %v", ycc)
	}
}
