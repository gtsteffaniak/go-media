package matroska

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVP8YPlaneMatchesFFmpeg(t *testing.T) {
	webm := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "webm_vp8.webm")
	if _, err := os.Stat(webm); err != nil {
		t.Skip("fixture missing")
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	ss := "0.51"
	out := filepath.Join(t.TempDir(), "ff.yuv")
	cmd := exec.Command(ff, "-y", "-hide_banner", "-loglevel", "error",
		"-i", webm, "-ss", ss, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "yuv420p", out)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	w, h := 1920, 1080
	if len(raw) < w*h*3/2 {
		t.Fatalf("short yuv %d", len(raw))
	}

	f, err := os.Open(webm)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	hdr, err := ProbeHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	pic, ok := VP8PictureAtSeek(f, st.Size(), hdr, 10, false)
	if !ok || pic == nil {
		t.Fatal("vp8 picture")
	}
	mse := 0.0
	n := 0
	maxDiff := 0.0
	for y := 0; y < h && y < pic.Height; y++ {
		for x := 0; x < w && x < pic.Width; x++ {
			yy := float64(pic.Y[y*pic.YStride+x])
			ffy := float64(raw[y*w+x])
			d := yy - ffy
			if ad := math.Abs(d); ad > maxDiff {
				maxDiff = ad
			}
			mse += d * d
			n++
		}
	}
	mse /= float64(n)
	psnr := 10 * math.Log10((255*255)/mse)
	t.Logf("Y PSNR=%.2f dB maxDiff=%.0f", psnr, maxDiff)
	if psnr < 45 {
		t.Errorf("Y plane PSNR %.2f dB < 45 (decoder or frame mismatch)", psnr)
	}
}
