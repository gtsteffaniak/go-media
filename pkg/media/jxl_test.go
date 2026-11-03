package media_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func TestThumbnailJXLUnsplash(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "zoltan_unsplash.jxl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(path, err) // the fixture is tracked; a missing file must fail, not skip
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	res, err := media.Thumbnail(f, st.Size(), media.ThumbnailOptions{
		MaxWidth:  256,
		MaxHeight: 256,
		Format:    "jpeg",
		Quality:   80,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) < 100 {
		t.Fatal("empty thumbnail")
	}
	if res.Width <= 0 || res.Height <= 0 {
		t.Fatalf("size %dx%d", res.Width, res.Height)
	}
}

func TestProbeJXLUnsplash(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "zoltan_unsplash.jxl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	info, err := media.Probe(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if info.FormatName != "jxl" {
		t.Fatalf("FormatName = %q, want jxl", info.FormatName)
	}
}
