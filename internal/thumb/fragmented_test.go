package thumb

import (
	"bytes"
	"fmt"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

// TestFragmentedH264MatchesFFmpeg seeks fragmented MP4 layouts ffmpeg writes
// (with and without mfra, default-base-is-moof, ismv) and checks the chosen
// keyframe time and pixels against ffmpeg's frame at that time.
func TestFragmentedH264MatchesFFmpeg(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	cases := []struct {
		name string
		args []string
	}{
		{"mfra", []string{"-movflags", "frag_keyframe+empty_moov"}},
		{"default_base_moof", []string{"-movflags", "frag_keyframe+empty_moov+default_base_moof"}},
		{"no mfra", []string{"-movflags", "frag_keyframe+empty_moov+skip_trailer"}},
		{"cmaf", []string{"-movflags", "frag_keyframe+empty_moov+default_base_moof+cmaf"}},
		{"ismv", []string{"-f", "ismv"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "f.mp4")
			args := append([]string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=s=320x180:r=25:d=8",
				"-c:v", "libx264", "-g", "25", "-keyint_min", "25"}, c.args...)
			if out, err := exec.Command(ffmpeg, append(args, src)...).CombinedOutput(); err != nil {
				t.Skipf("encode: %v %s", err, out)
			}
			data, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			r := bytes.NewReader(data)
			for _, pct := range []float64{10, 50, 90} {
				ts, ok := isobmff.H264SyncTimeSec(r, int64(len(data)), 0, pct)
				if !ok {
					t.Fatalf("%v%%: no sync time", pct)
				}
				// GOP is 1 s and the file 8 s: the keyframe at or before the
				// target is floor(target).
				if want := math.Floor(8 * pct / 100); math.Abs(ts-want) > 1e-6 {
					t.Fatalf("%v%%: keyframe at %.3fs, want %.3fs", pct, ts, want)
				}
				img, err := isobmff.H264ThumbnailRGBA(r, int64(len(data)), pct, false)
				if err != nil || img == nil {
					t.Fatalf("%v%%: decode: %v", pct, err)
				}
				goPNG := filepath.Join(dir, "go.png")
				if err := png.Encode(mustCreate(goPNG), img); err != nil {
					t.Fatal(err)
				}
				ffPNG := filepath.Join(dir, "ff.png")
				if out, err := exec.Command(ffmpeg, "-y", "-v", "error", "-ss", fmt.Sprintf("%.6f", ts), "-i", src, "-frames:v", "1", ffPNG).CombinedOutput(); err != nil {
					t.Fatalf("ffmpeg: %v %s", err, out)
				}
				psnr, err := imageFilePSNR(ffmpeg, ffPNG, goPNG)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("%s %v%%: keyframe %.3fs PSNR %.2f dB", c.name, pct, ts, psnr)
				if psnr < 40 {
					t.Fatalf("PSNR %.2f dB < 40", psnr)
				}
			}
		})
	}
}
