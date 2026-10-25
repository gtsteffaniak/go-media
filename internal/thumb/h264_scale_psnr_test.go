package thumb

import (
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

func TestH264AreaScaleMatchesFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(err)
	}
	st, _ := f.Stat()
	src, err := isobmff.H264ThumbnailRGBA(f, st.Size(), 10, false)
	if err != nil || src == nil {
		t.Fatalf("decode: %v", err)
	}
	tw, th := fitThumbSize(src.Bounds().Dx(), src.Bounds().Dy(), 1280, 720)
	goScaled := scaleAreaRGBA(src, tw, th)
	dir := t.TempDir()
	fullPNG := filepath.Join(dir, "full.png")
	if err := png.Encode(mustCreate(fullPNG), src); err != nil {
		t.Fatal(err)
	}
	ffPNG := filepath.Join(dir, "ff.png")
	vf := fmt.Sprintf("scale=w=%d:h=%d:force_original_aspect_ratio=decrease:flags=area,scale=trunc(iw/2)*2:trunc(ih/2)*2", 1280, 720)
	if err := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", fullPNG, "-vf", vf, ffPNG).Run(); err != nil {
		t.Fatal(err)
	}
	goPNG := filepath.Join(dir, "go.png")
	if err := png.Encode(mustCreate(goPNG), goScaled); err != nil {
		t.Fatal(err)
	}
	psnr, err := imageFilePSNR(ffmpeg, ffPNG, goPNG)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scaled PNG PSNR go area vs ffmpeg area: %.2f dB (%dx%d)", psnr, tw, th)
}

func mustCreate(path string) *os.File {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	return f
}

func imageFilePSNR(ffmpeg, a, b string) (float64, error) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "info",
		"-i", a, "-i", b, "-lavfi", "psnr", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "PSNR") {
		return 0, fmt.Errorf("%w: %s", err, text)
	}
	return parsePSNR(text)
}

func parsePSNR(text string) (float64, error) {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "average:") && strings.Contains(line, "PSNR") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "average:") {
					v, err := strconv.ParseFloat(strings.TrimPrefix(field, "average:"), 64)
					if err == nil && v > 0 {
						return v, nil
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("psnr not found")
}
