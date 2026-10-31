package ts

import (
	"os"
	"testing"
)

func TestProbeMPEG2TSFixture(t *testing.T) {
	path := "../../.assets/video/mpegts_mpeg2.ts"
	if _, err := os.Stat(path); err != nil {
		t.Skip("compare asset not generated")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	m, err := Probe(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Streams) != 1 {
		for _, s := range m.Streams {
			t.Logf("stream type=0x%02x codec=%s pid=0x%x", s.StreamType, s.Codec, s.PID)
		}
		t.Fatalf("streams=%d want 1", len(m.Streams))
	}
	if m.Streams[0].Codec != "mpeg2video" {
		t.Fatalf("codec=%s", m.Streams[0].Codec)
	}
	if m.Duration < 4.5 || m.Duration > 5.5 {
		t.Fatalf("duration=%v want ~5.1s", m.Duration)
	}
}
