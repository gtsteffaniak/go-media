package isobmff

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestH264BaselineRGBSwscaleMatchesGo(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264_baseline_1080.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(err)
	}
	st, _ := f.Stat()
	img, ok := H264ThumbnailRGBA(f, st.Size(), 10, false)
	if !ok {
		t.Fatal("decode failed")
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	dir := t.TempDir()
	yuvPath := filepath.Join(dir, "k.yuv")
	if err := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-skip_frame", "nokey", "-ss", "0.5", "-i", path,
		"-frames:v", "1", "-pix_fmt", "yuv420p", yuvPath).Run(); err != nil {
		t.Fatal(err)
	}
	rgbFF := filepath.Join(dir, "ff.rgb")
	size := fmt.Sprintf("%dx%d", w, h)
	if err := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", size, "-i", yuvPath,
		"-pix_fmt", "rgb24", rgbFF).Run(); err != nil {
		t.Fatal(err)
	}
	gb := make([]byte, w*h*3)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, y)
			i := (y*w + x) * 3
			gb[i], gb[i+1], gb[i+2] = c.R, c.G, c.B
		}
	}
	rgbGo := filepath.Join(dir, "go.rgb")
	if err := os.WriteFile(rgbGo, gb, 0o644); err != nil {
		t.Fatal(err)
	}
	psnr, err := rawRGBPSNR(ffmpeg, rgbFF, rgbGo, w, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("RGB PSNR ffmpeg swscale(yuv420p) vs go frameToRGBA @0.5s: %.2f dB", psnr)

	rgbDirect := filepath.Join(dir, "direct.rgb")
	if err := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-skip_frame", "nokey", "-ss", "0.5", "-i", path,
		"-frames:v", "1", "-pix_fmt", "rgb24", rgbDirect).Run(); err != nil {
		t.Fatal(err)
	}
	psnr2, err := rawRGBPSNR(ffmpeg, rgbDirect, rgbGo, w, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("RGB PSNR ffmpeg h264->rgb24 vs go frameToRGBA @0.5s: %.2f dB", psnr2)
}

func rawRGBPSNR(ffmpeg, a, b string, w, h int) (float64, error) {
	size := fmt.Sprintf("%dx%d", w, h)
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "info",
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-s", size, "-i", a,
		"-f", "rawvideo", "-pix_fmt", "rgb24", "-s", size, "-i", b,
		"-lavfi", "psnr", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "PSNR") {
		return 0, fmt.Errorf("%w: %s", err, text)
	}
	return parsePSNRAverage(text)
}

func parsePSNRAverage(text string) (float64, error) {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "average:") && strings.Contains(line, "PSNR") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "average:") {
					v, err := strconv.ParseFloat(strings.TrimPrefix(field, "average:"), 64)
					if err == nil && v > 0 {
						if v >= 1e10 {
							return 100, nil
						}
						return v, nil
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("psnr not found")
}
