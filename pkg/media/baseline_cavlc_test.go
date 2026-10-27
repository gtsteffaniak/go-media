package media

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// baselineCAVLCffmpegSHA256 is the SHA-256 of ffmpeg's I420 decode (loop filter on) of
// testdata/baseline_cavlc_128x128.mp4, a real-content libx264 Constrained Baseline (CAVLC)
// IDR frame. Regenerate with:
//
//	ffmpeg -i testdata/baseline_cavlc_128x128.mp4 -frames:v 1 -pix_fmt yuv420p -f rawvideo out.yuv
const baselineCAVLCffmpegSHA256 = "6149432c430b183258275804da8d3fe3855d39eb64110f4b6b3271d70a15d751"

// TestBaselineCAVLCMatchesFFmpeg decodes Baseline (CAVLC), which is enabled by default, and
// requires the output to be bit-exact with ffmpeg. The CAVLC path once decoded garbage
// (stale run_before values from a reused scratch buffer) while every CABAC stream was
// exact, and it went unnoticed because Baseline was rejected before decoding.
func TestBaselineCAVLCMatchesFFmpeg(t *testing.T) {
	f, err := os.Open("../../testdata/baseline_cavlc_128x128.mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	yuv, w, h, err := H264KeyframeYUV420(f, st.Size(), 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if w != 128 || h != 128 {
		t.Fatalf("size %dx%d, want 128x128", w, h)
	}
	sum := sha256.Sum256(yuv)
	if got := hex.EncodeToString(sum[:]); got != baselineCAVLCffmpegSHA256 {
		t.Fatalf("Baseline CAVLC decode differs from ffmpeg: sha256 %s, want %s", got, baselineCAVLCffmpegSHA256)
	}
}
