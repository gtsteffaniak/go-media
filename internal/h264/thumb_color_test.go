package h264

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/frame"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// ffmpeg's swscale defaults untagged input to BT.601 at every resolution.
func TestThumbColorFromFrameHDDefault(t *testing.T) {
	f := frame.NewFrame(1920, 1080)
	cs, rng := ThumbColorFromFrame(f)
	if cs != yuv.BT601 {
		t.Fatalf("unspecified HD matrix: got %v want bt601", cs)
	}
	if rng != yuv.LimitedRange {
		t.Fatalf("range: got %v want limited", rng)
	}
}

func TestThumbColorFromFrameSDDefault(t *testing.T) {
	f := frame.NewFrame(640, 480)
	cs, _ := ThumbColorFromFrame(f)
	if cs != yuv.BT601 {
		t.Fatalf("unspecified SD matrix: got %v want bt601", cs)
	}
}

func TestThumbColorFromFrameVUI(t *testing.T) {
	f := frame.NewFrame(1920, 1080)
	f.ColorDescriptionValid = true
	f.MatrixCoefficients = 5
	cs, _ := ThumbColorFromFrame(f)
	if cs != yuv.BT601 {
		t.Fatalf("explicit bt601: got %v", cs)
	}
}

func TestThumbColorFromFrameExplicit709FullRange(t *testing.T) {
	f := frame.NewFrame(640, 480)
	f.ColorDescriptionValid = true
	f.MatrixCoefficients = 1
	f.VideoFullRangeFlag = true
	cs, rng := ThumbColorFromFrame(f)
	if cs != yuv.BT709 || rng != yuv.FullRange {
		t.Fatalf("got %v/%v want bt709/full", cs, rng)
	}
}

// matrix_coefficients 2 means "unspecified"; ffmpeg falls back to BT.601.
func TestThumbColorFromFrameUnspecifiedTag(t *testing.T) {
	f := frame.NewFrame(1920, 1080)
	f.ColorDescriptionValid = true
	f.MatrixCoefficients = 2
	cs, _ := ThumbColorFromFrame(f)
	if cs != yuv.BT601 {
		t.Fatalf("mc=2: got %v want bt601", cs)
	}
}

func TestMatchFFmpegYUVOutput(t *testing.T) {
	t709 := &ThumbYCbCr{CS: yuv.BT709, RNG: yuv.FullRange}
	t709.MatchFFmpegYUVOutput()
	if t709.CS != yuv.BT601 || t709.RNG != yuv.FullRange {
		t.Fatalf("got %v/%v want bt601/full", t709.CS, t709.RNG)
	}
}
