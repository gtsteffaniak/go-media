package ts_test

import (
	"os"
	"testing"

	ts "github.com/gtsteffaniak/go-media/internal/mpegts"
)

func TestTSProbeFixture(t *testing.T) {
	f, err := os.Open("../../testdata/mpeg2_176x144.ts")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	meta, err := ts.Probe(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("video pid=%x type=%x dur=%v streams=%d", meta.VideoPID, meta.VideoTyp, meta.Duration, len(meta.Streams))
	y, err := ts.ThumbnailYCbCr(f, st.Size(), meta, 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if y == nil {
		t.Fatal("nil")
	}
}
