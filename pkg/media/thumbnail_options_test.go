package media_test

import (
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func TestThumbnailPresetSmall(t *testing.T) {
	opt := media.OptionsForPreset(media.ThumbnailPresetSmall, 10)
	if opt.MaxWidth != 256 || opt.Quality != 76 {
		t.Fatalf("small preset: %+v", opt)
	}
	if opt.Filter != media.ThumbnailFilterArea {
		t.Fatalf("expected area filter, got %s", opt.Filter)
	}
}

func TestThumbnailPresetLarge(t *testing.T) {
	opt := media.OptionsForPreset(media.ThumbnailPresetLarge, 10)
	if opt.MaxWidth != 1280 || opt.Quality != 90 {
		t.Fatalf("large preset: %+v", opt)
	}
	if opt.Filter != media.ThumbnailFilterArea {
		t.Fatalf("expected area filter for large preset, got %s", opt.Filter)
	}
}

func TestThumbnailSeekAlias(t *testing.T) {
	f, size := openFixture(t, "frame.mjpeg.mp4")
	defer f.Close()
	res, err := media.Thumbnail(f, size, media.ThumbnailOptions{
		MaxWidth: 48, Seek: 10, Quality: 90, Format: "jpeg",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) < 100 {
		t.Fatalf("short thumbnail")
	}
}
