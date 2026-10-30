package isobmff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// TestH264ThumbnailHonorsVUIColour checks that the SPS VUI range and matrix
// reach the thumbnail colour selection (ffmpeg honors both tags).
func TestH264ThumbnailHonorsVUIColour(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "full709_h264.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("fixtures missing, run scripts/generate-fixtures.sh")
	}
	defer f.Close()
	st, _ := f.Stat()
	img, err := H264ThumbnailYCbCr(f, st.Size(), 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if img.CS != yuv.BT709 || img.RNG != yuv.FullRange {
		t.Fatalf("got %v/%v, want bt709/full", img.CS, img.RNG)
	}
}

// TestH264ThumbnailOpenGOPKeyframe seeks onto a non-IDR I keyframe of an
// x264 open-GOP stream, which used to fail with "no decodable picture".
func TestH264ThumbnailOpenGOPKeyframe(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "opengop_h264.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("fixtures missing, run scripts/generate-fixtures.sh")
	}
	defer f.Close()
	st, _ := f.Stat()
	for _, p := range []float64{40, 70, 90} {
		img, err := H264ThumbnailYCbCr(f, st.Size(), p, true)
		if err != nil || img == nil {
			t.Fatalf("seek %v%%: %v", p, err)
		}
	}
}
