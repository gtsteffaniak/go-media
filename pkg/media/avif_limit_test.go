package media_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func TestThumbnailAVIFMaxSourcePixels(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "i8_1080.avif")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	_, err = media.Thumbnail(f, st.Size(), media.ThumbnailOptions{
		MaxWidth: 256, MaxHeight: 256, Format: "jpeg", MaxSourcePixels: 1000,
	})
	if !errors.Is(err, media.ErrUnsupported) || !strings.Contains(err.Error(), "1000") {
		t.Fatalf("want ErrUnsupported naming cap, got %v", err)
	}
}
