package mpeg2video_test

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/mpeg2video"
)

func TestDecodeKeyframeES(t *testing.T) {
	b, err := os.ReadFile("testdata/intra_64x48.m2v")
	if err != nil {
		t.Fatal(err)
	}
	pic, err := mpeg2video.DecodeKeyframe(b)
	if err != nil {
		t.Fatal(err)
	}
	if pic.Width != 64 || pic.Height != 48 {
		t.Fatalf("size %dx%d", pic.Width, pic.Height)
	}
}

func FuzzDecodeKeyframe(f *testing.F) {
	f.Add([]byte{0, 0, 1, 0xb3, 0, 0x10, 0, 0x10})
	f.Fuzz(func(t *testing.T, b []byte) {
		_, _ = mpeg2video.DecodeKeyframe(b)
	})
}

func TestDecode176ES(t *testing.T) {
	b, err := os.ReadFile("testdata/intra_176x144.m2v")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mpeg2video.DecodeKeyframe(b); err != nil {
		t.Fatal(err)
	}
}
