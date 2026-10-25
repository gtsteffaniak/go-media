package media_test

import (
	"testing"

	"github.com/gtsteffaniak/go-media"
)

func TestThumbnailMJPEGInMP4(t *testing.T) {
	f, size := openFixture(t, "frame.mjpeg.mp4")
	defer f.Close()
	res, err := media.Thumbnail(f, size, media.ThumbnailOptions{MaxWidth: 48, Format: "jpeg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Data) < 100 || res.MIME != "image/jpeg" {
		t.Fatalf("unexpected thumbnail: mime=%s len=%d", res.MIME, len(res.Data))
	}
}

func TestThumbnailFLACArt(t *testing.T) {
	f, size := openFixture(t, "tone.flac")
	defer f.Close()
	_, err := media.Thumbnail(f, size, media.ThumbnailOptions{MaxWidth: 48, PreferEmbedded: true})
	if err == nil {
		return // ok if ffmpeg embedded art in flac
	}
	// tone.flac fixture has no embedded picture — unsupported is fine
	if err != media.ErrUnsupported {
		t.Fatalf("expected unsupported or success, got %v", err)
	}
}
