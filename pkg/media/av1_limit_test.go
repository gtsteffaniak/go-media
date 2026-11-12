package media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

)

// TestAV1OverLimitIsUnsupported checks that an AV1 stream above the thumbnail
// frame-size limit returns ErrUnsupportedProfile, the error callers such as
// FileBrowser treat as "use ffmpeg instead". The limit is lowered below the
// 4K fixture's size so no 8K fixture is needed.
func TestAV1OverLimitIsUnsupported(t *testing.T) {
	path := filepath.Join("..", "..", "test", "data", "av1", "s8_4k.mkv")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(path, err)
	}
	defer f.Close()
	st, _ := f.Stat()

	_, err = Thumbnail(f, st.Size(), ThumbnailOptions{
		MaxWidth: 320, MaxHeight: 320, Format: "jpeg",
		MaxAV1FrameLongSide: 1920, MaxAV1FrameShortSide: 1080,
	})
	if !errors.Is(err, ErrUnsupportedProfile) {
		t.Fatalf("want ErrUnsupportedProfile, got %v", err)
	}
}
