package media

import (
	"os"
	"path/filepath"
	"testing"
)

// TestH264KeyframeMKV decodes the compare-harness MKV (H.264 stream copy of
// the reference clip) when it has been generated locally.
func TestH264KeyframeMKV(t *testing.T) {
	p := filepath.Join("..", "..", "cmd", "gomedia-compare", ".assets", "video", "mkv_h264.mkv")
	f, err := os.Open(p)
	if err != nil {
		t.Skip("fixture not generated:", err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	sec, err := H264KeyframeTimeSec(f, st.Size(), 50)
	if err != nil || sec <= 0 {
		t.Fatalf("keyframe time %v err %v", sec, err)
	}
	b, w, h, err := H264KeyframeYUV420(f, st.Size(), 50, true)
	if err != nil {
		t.Fatal(err)
	}
	if w != 1920 || h != 1080 || len(b) != w*h*3/2 {
		t.Fatalf("got %dx%d len %d", w, h, len(b))
	}
}
