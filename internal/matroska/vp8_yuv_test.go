package matroska

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const vp8TestSeekPercent = 10

// openVP8Fixture opens the compare harness VP8 fixture and probes it.
func openVP8Fixture(t *testing.T) (*os.File, int64, *Header) {
	t.Helper()
	webm := filepath.Join("..", "..", "cmd", "gomedia-compare", ".assets", "video", "webm_vp8.webm")
	if _, err := os.Stat(webm); err != nil {
		t.Skip("fixture missing (run make compare to generate it)")
	}
	f, err := os.Open(webm)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	hdr, err := ProbeHeader(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	return f, st.Size(), hdr
}

// vp8SeekTargets returns the exact 10% target and the keyframe at or before
// it, both in milliseconds, using the same frame index the decoder uses.
func vp8SeekTargets(t *testing.T, f *os.File, size int64, hdr *Header) (targetMs, keyMs int64) {
	t.Helper()
	targetMs = int64(float64(hdr.durationMs()) * vp8TestSeekPercent / 100.0)
	ix, err := indexSegment(f, size)
	if err != nil {
		t.Fatal(err)
	}
	track, _, ok := videoTrackNumber(ix.tracks, "V_VP8")
	if !ok {
		t.Fatal("no VP8 track")
	}
	frames, err := collectSeekFrames(f, ix, track, hdr.timecodeScale(), targetMs, true)
	if err != nil || len(frames) == 0 {
		t.Fatalf("frames: %v", err)
	}
	keyMs = frames[0].timestampMs
	for _, fr := range frames {
		if fr.keyframe && fr.timestampMs <= targetMs {
			keyMs = fr.timestampMs
		}
	}
	return targetMs, keyMs
}

// ffmpegYAt decodes the first frame at or after ms with ffmpeg and returns its
// Y plane (yuv420p, no range conversion).
func ffmpegYAt(t *testing.T, path string, ms int64, w, h int) []byte {
	t.Helper()
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("no ffmpeg")
	}
	out := filepath.Join(t.TempDir(), "ff.yuv")
	sel := fmt.Sprintf("select=gte(t\\,%.3f)", float64(ms)/1000-0.0005)
	cmd := exec.Command(ff, "-y", "-hide_banner", "-loglevel", "error",
		"-i", path, "-vf", sel, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "yuv420p", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, b)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < w*h*3/2 {
		t.Fatalf("short yuv %d", len(raw))
	}
	return raw
}

func checkVP8Y(t *testing.T, exact bool) {
	var wantMs int64
	t.Helper()
	f, size, hdr := openVP8Fixture(t)
	if exact {
		wantMs, _ = vp8SeekTargets(t, f, size, hdr)
	} else {
		_, wantMs = vp8SeekTargets(t, f, size, hdr)
	}
	pic, ok := VP8PictureAtSeek(f, size, hdr, vp8TestSeekPercent, exact)
	if !ok || pic == nil {
		t.Fatal("vp8 picture")
	}
	w, h := pic.Width, pic.Height
	raw := ffmpegYAt(t, f.Name(), wantMs, w, h)
	mse, n, maxDiff := 0.0, 0, 0.0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := float64(pic.Y[y*pic.YStride+x]) - float64(raw[y*w+x])
			maxDiff = math.Max(maxDiff, math.Abs(d))
			mse += d * d
			n++
		}
	}
	mse /= float64(n)
	psnr := math.Inf(1)
	if mse > 0 {
		psnr = 10 * math.Log10((255*255)/mse)
	}
	t.Logf("exact=%v frame at %dms: Y PSNR=%.2f dB maxDiff=%.0f", exact, wantMs, psnr, maxDiff)
	if psnr < 45 {
		t.Errorf("Y plane PSNR %.2f dB < 45 (decoder or frame mismatch)", psnr)
	}
}

// TestVP8YPlaneMatchesFFmpeg checks the exact seek against ffmpeg's frame at
// the 10% timestamp.
func TestVP8YPlaneMatchesFFmpeg(t *testing.T) { checkVP8Y(t, true) }

// TestVP8KeyframeSnapMatchesFFmpeg checks the default seek returns the
// keyframe at or before the 10% timestamp, matching ffmpeg's frame there.
func TestVP8KeyframeSnapMatchesFFmpeg(t *testing.T) {
	f, size, hdr := openVP8Fixture(t)
	targetMs, keyMs := vp8SeekTargets(t, f, size, hdr)
	if keyMs > targetMs {
		t.Fatalf("keyframe %dms after target %dms", keyMs, targetMs)
	}
	checkVP8Y(t, false)
}
