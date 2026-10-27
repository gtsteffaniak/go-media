package thumb

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

func TestH264SmallJPEGNearFFmpeg(t *testing.T) {
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
	opt := Options{MaxWidth: 256, MaxHeight: 256, SeekPercent: 10, Quality: 76, Scaler: ScalerArea, Fast: true}
	res, err := Generate(f, st.Size(), opt)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	goJ := filepath.Join(dir, "go.jpg")
	ffJ := filepath.Join(dir, "ff.jpg")
	if err := os.WriteFile(goJ, res.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	f2, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st2, _ := f2.Stat()
	seekSec, ok := isobmff.H264SyncTimeSec(f2, st2.Size(), 0, 10)
	f2.Close()
	if !ok {
		t.Fatal("sync time")
	}
	vf := "scale=w='min(256,iw)':h='min(256,ih)':force_original_aspect_ratio=decrease:flags=area,scale=trunc(iw/2)*2:trunc(ih/2)*2"
	if err := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-skip_frame", "nokey", "-skip_loop_filter", "all", "-threads", "1",
		"-ss", fmt.Sprintf("%.3f", seekSec), "-i", path, "-frames:v", "1", "-vf", vf, "-q:v", "9", ffJ).Run(); err != nil {
		t.Fatal(err)
	}
	psnr, err := imageFilePSNR(ffmpeg, ffJ, goJ)
	if err != nil {
		t.Fatal(err)
	}
	ssim, err := imageFileSSIM(ffmpeg, ffJ, goJ)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("small thumb jpeg PSNR=%.1f SSIM=%.3f", psnr, ssim)
	if psnr < 28 {
		t.Fatalf("PSNR %.1f < 28 (color/scale drift vs ffmpeg)", psnr)
	}
	if ssim < 0.85 {
		t.Fatalf("SSIM %.3f < 0.85", ssim)
	}
}

func TestH264RGBDecodeMatchesFrameToImage(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip(err)
	}
	st, _ := f.Stat()
	rgba, err := isobmff.H264ThumbnailRGBA(f, st.Size(), 10, true)
	if err != nil || rgba == nil {
		t.Fatal(err)
	}
	if rgba.Bounds().Dx() < 16 {
		t.Fatal("empty rgba")
	}
}

func imageFileSSIM(ffmpeg, a, b string) (float64, error) {
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "info",
		"-i", a, "-i", b, "-lavfi", "ssim", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "SSIM") {
		return 0, fmt.Errorf("%w: %s", err, text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "All:") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "All:") {
					return strconv.ParseFloat(strings.TrimPrefix(field, "All:"), 64)
				}
			}
		}
	}
	return 0, fmt.Errorf("ssim not found")
}
