package mpeg2video_test

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/mpeg2video"
)

func TestDecodeIFrameIndexFallback(t *testing.T) {
	es, err := os.ReadFile("testdata/intra_176x144.m2v")
	if err != nil {
		t.Fatal(err)
	}
	// Fixture has fewer decodable I-pictures than this index; seek clamps to last I-picture.
	pic, err := mpeg2video.DecodeIFrameIndex(es, 8)
	if err != nil {
		t.Fatal(err)
	}
	if pic.Width != 176 || pic.Height != 144 {
		t.Fatalf("size %dx%d", pic.Width, pic.Height)
	}
}
