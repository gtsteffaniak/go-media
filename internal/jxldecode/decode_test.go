package jxldecode_test

import (
	"bytes"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/jxldecode"
)

func testDataPath(name string) string {
	return filepath.Join("..", "..", "test", "data", name)
}

func TestDecodePQGradient(t *testing.T) {
	path := testDataPath("pq_gradient.jxl")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	img, err := jxldecode.Decode(f, st.Size(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatal("empty image")
	}
}

func TestDecodeUnsplashWithResize(t *testing.T) {
	path := testDataPath("zoltan_unsplash.jxl")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	img, err := jxldecode.Decode(f, st.Size(), 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() > 256 || b.Dy() > 256 {
		t.Fatalf("bounds %dx%d exceed 256", b.Dx(), b.Dy())
	}
}

func TestDecodeUnsplashJPEGRoundTrip(t *testing.T) {
	path := testDataPath("zoltan_unsplash.jxl")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	img, err := jxldecode.Decode(f, st.Size(), 640, 640)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() < 100 {
		t.Fatal("expected non-trivial JPEG output")
	}
}
