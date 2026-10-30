package thumb

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

// TestAVC3MatchesFFmpeg decodes avc3 (in-band SPS/PPS) keyframes, with and
// without an avcC box, and compares them with ffmpeg's frame at the same time.
func TestAVC3MatchesFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "avc3.mp4")
	if out, err := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=4",
		"-c:v", "libx264", "-g", "25", "-x264-params", "repeat-headers=1", "-tag:v", "avc3", src).CombinedOutput(); err != nil {
		t.Skipf("libx264 unavailable: %v %s", err, out)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	// Renaming avcC to free leaves an avc3 entry with no decoder
	// configuration, the layout some live packagers write.
	noAVCC := bytes.Replace(append([]byte(nil), data...), []byte("avcC"), []byte("free"), 1)
	for name, b := range map[string][]byte{"with avcC": data, "no avcC": noAVCC} {
		t.Run(name, func(t *testing.T) {
			r := bytes.NewReader(b)
			img, err := isobmff.H264ThumbnailRGBA(r, int64(len(b)), 50, false)
			if err != nil || img == nil {
				t.Fatalf("decode: %v", err)
			}
			ts, ok := isobmff.H264SyncTimeSec(r, int64(len(b)), 0, 50)
			if !ok {
				t.Fatal("no sync time")
			}
			goPNG := filepath.Join(t.TempDir(), "go.png")
			if err := png.Encode(mustCreate(goPNG), img); err != nil {
				t.Fatal(err)
			}
			ffPNG := filepath.Join(t.TempDir(), "ff.png")
			if out, err := exec.Command(ffmpeg, "-v", "error", "-i", src, "-ss", fmt.Sprintf("%.6f", ts), "-frames:v", "1", ffPNG).CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg: %v %s", err, out)
			}
			psnr, err := imageFilePSNR(ffmpeg, ffPNG, goPNG)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("avc3 %s: PSNR %.2f dB vs ffmpeg at %.3fs", name, psnr, ts)
			if psnr < 40 {
				t.Fatalf("PSNR %.2f dB < 40", psnr)
			}
		})
	}
}
