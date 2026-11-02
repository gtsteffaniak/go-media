package sniff_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/sniff"
)

func TestDetectJXLContainer(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "zoltan_unsplash.jxl")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	id, err := sniff.Detect(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if id != format.JXL {
		t.Fatalf("Detect() = %q, want jxl", id)
	}
}
