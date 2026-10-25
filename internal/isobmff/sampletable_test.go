package isobmff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func TestSampleRefsAndKeyframes(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264.mp4")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("comparison fixture missing")
	}
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	r := ioat.New(f, st.Size())
	refs, err := SampleRefs(r, st.Size(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) == 0 {
		t.Fatal("expected samples")
	}
	if refs[0].Offset <= 0 || refs[0].Size <= 0 {
		t.Fatalf("bad first sample: %+v", refs[0])
	}
	sync, err := SyncSampleNumbers(r, st.Size(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sync) == 0 {
		t.Fatal("expected sync samples")
	}
	kf, err := KeyframeSamples(r, st.Size(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(kf) == 0 {
		t.Fatal("expected keyframe refs")
	}
}
