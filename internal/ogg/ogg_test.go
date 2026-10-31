package ogg

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func TestVorbisSampleRateFixture(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "audio", "ogg_vorbis.ogg")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("comparison fixture missing")
	}
	if got := vorbisSampleRate(data); got != 44100 {
		t.Fatalf("vorbisSampleRate %d len %d", got, len(data))
	}
}

func TestDurationVorbisFixture(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "audio", "ogg_vorbis.ogg")
	f, err := os.Open(path)
	if err != nil {
		t.Skip("comparison fixture missing")
	}
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	ra := ioat.New(f, st.Size())
	head, _ := ioat.ReadAll(ra, st.Size(), 0, min64(st.Size(), 128*1024))
	sr := sampleRateFromHead(head)
	readLen := st.Size()
	if readLen > 512*1024 {
		readLen = 512 * 1024
	}
	tail, _ := ioat.ReadAll(ra, st.Size(), st.Size()-readLen, readLen)
	g, ok := maxGranule(tail)
	if !ok || g < 200000 {
		t.Fatalf("maxGranule=%d ok=%v sr=%d tailLen=%d", g, ok, sr, len(tail))
	}
	dr, err := Duration(ra, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	sec := dr.Duration.Seconds()
	if sec < 4.5 || sec > 5.5 {
		t.Fatalf("duration %.3fs, want ~5s", sec)
	}
	_ = time.Second
}
