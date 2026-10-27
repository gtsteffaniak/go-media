package thumb

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

func TestH264LargeJPEGNearFFmpeg(t *testing.T) {
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
	opt := Options{MaxWidth: 1280, MaxHeight: 720, SeekPercent: 10, Quality: 90, Scaler: ScalerArea}
	res, err := Generate(f, st.Size(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if useH264RGBPipeline(opt) {
		t.Fatal("large preset must use YCbCr pipeline")
	}
	dir := t.TempDir()
	goJ := filepath.Join(dir, "go.jpg")
	ffJ := filepath.Join(dir, "ff.jpg")
	if err := os.WriteFile(goJ, res.Data, 0o644); err != nil {
		t.Fatal(err)
	}
	f2, _ := os.Open(path)
	st2, _ := f2.Stat()
	seekSec, ok := isobmff.H264SyncTimeSec(f2, st2.Size(), 0, 10)
	f2.Close()
	if !ok {
		t.Fatal("sync time")
	}
	qv := 31 - (90*29)/100
	vf := "scale=w='min(1280,iw)':h='min(720,ih)':force_original_aspect_ratio=decrease:flags=area,scale=trunc(iw/2)*2:trunc(ih/2)*2"
	if err := exec.Command(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-skip_frame", "nokey", "-skip_loop_filter", "all", "-threads", "1",
		"-ss", fmt.Sprintf("%.3f", seekSec), "-i", path, "-frames:v", "1", "-vf", vf,
		"-q:v", fmt.Sprintf("%d", qv), ffJ).Run(); err != nil {
		t.Fatal(err)
	}
	psnr, err := imageFilePSNR(ffmpeg, ffJ, goJ)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("large thumb jpeg PSNR=%.1f", psnr)
	if psnr < 28 {
		t.Fatalf("PSNR %.1f < 28", psnr)
	}
}
