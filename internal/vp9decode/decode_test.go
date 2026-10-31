package vp9decode_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/vp9decode"
)

func TestVP9PictureAtSeekWebM(t *testing.T) {
	path := vp9Fixture(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	hdr, err := matroska.ProbeHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	pic, err := matroska.VP9PictureAtSeek(f, st.Size(), hdr, 10)
	if err != nil {
		t.Fatal(err)
	}
	if pic == nil {
		t.Fatal("no picture")
	}
	if pic.Width < 16 || pic.Height < 16 {
		t.Fatalf("unexpected size %dx%d", pic.Width, pic.Height)
	}
	ycc := vp9decode.PictureThumbYCbCr(pic)
	if ycc == nil || len(ycc.YCbCr.Y) == 0 {
		t.Fatal("empty Y plane")
	}
}

func vp9Fixture(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("GOMEDIA_VP9_WEBM"); p != "" {
		return p
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "vp9.webm")
	out, err := exec.Command("ffmpeg", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-t", "2", "-c:v", "libvpx-vp9", "-b:v", "800k", "-an", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, out)
	}
	return path
}
