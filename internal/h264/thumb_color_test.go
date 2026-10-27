package h264

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/frame"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

func TestThumbColorFromFrameHDDefault(t *testing.T) {
	f := frame.NewFrame(1920, 1080)
	cs, rng := ThumbColorFromFrame(f)
	if cs != yuv.BT709 {
		t.Fatalf("unspecified HD matrix: got %v want bt709", cs)
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
