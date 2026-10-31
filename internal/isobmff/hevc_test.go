package isobmff_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

func TestHEVCThumbnailYCbCrMP4(t *testing.T) {
	path := hevcMP4Fixture(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	src, err := isobmff.OpenThumbSourceErr(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	ycc, err := src.HEVCThumbnailYCbCr(10)
	if err != nil {
		t.Fatal(err)
	}
	if ycc == nil || ycc.Bounds().Dx() < 16 {
		t.Fatalf("ycc=%v", ycc)
	}
}

func hevcMP4Fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hevc.mp4")
	clip := filepath.Join("..", "..", "test", "data", "reference_1080_10s.mp4")
	out, err := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-i", clip, "-t", "2", "-vf", "scale=640:360", "-c:v", "libx265", "-tag:v", "hvc1",
		"-pix_fmt", "yuv420p", "-x265-params", "keyint=30:min-keyint=30", "-an", path).CombinedOutput()
	if err != nil {
		t.Skip("ffmpeg libx265: ", string(out))
	}
	return path
}
