package media_test

import (
	"testing"

	"github.com/gtsteffaniak/go-media"
)

func TestThumbnailVP8WebM(t *testing.T) {
	f, size := openFixture(t, "blue.webm")
	defer f.Close()
	res, err := media.Thumbnail(f, size, media.ThumbnailOptions{MaxWidth: 64, SeekPercent: 50, Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) < 100 {
		t.Fatalf("short thumbnail %d", len(res.Data))
	}
}
