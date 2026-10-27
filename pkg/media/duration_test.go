package media_test

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// bytesAt is a minimal io.ReaderAt over a byte slice.
type bytesAt struct {
	b []byte
}

// ReadAt implements io.ReaderAt.
func (b bytesAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.b)) {
		return 0, os.ErrInvalid
	}
	n := copy(p, b.b[off:])
	return n, nil
}

// TestUnsupportedErrorKeepsDetail is a test.
func TestUnsupportedErrorKeepsDetail(t *testing.T) {
	wav := []byte("RIFF\x24\x00\x00\x00WAVEfmt \x10\x00\x00\x00")
	_, err := media.Duration(bytesAt{b: wav}, int64(len(wav)))
	var u *media.UnsupportedError
	if !errors.As(err, &u) || u.Reason == "" {
		t.Fatalf("expected detailed unsupported, got %v", err)
	}
}

// openFixture opens a testdata file or skips the test.
func openFixture(t *testing.T, name string) (*os.File, int64) {
	f, err := os.Open("../../testdata/" + name)
	if err != nil {
		t.Skip("missing fixture, run scripts/generate-fixtures.sh:", err)
	}
	st, _ := f.Stat()
	return f, st.Size()
}

// TestDurationWAV is a test.
func TestDurationWAV(t *testing.T) {
	f, size := openFixture(t, "silence.wav")
	defer f.Close()
	dr, err := media.Duration(f, size)
	if err != nil {
		t.Fatal(err)
	}
	if dr.Duration < 900*time.Millisecond || dr.Duration > 1100*time.Millisecond {
		t.Fatalf("expected ~1s, got %v", dr.Duration)
	}
	if dr.Source != media.SourceHeader {
		t.Fatalf("expected header source, got %q", dr.Source)
	}
}

// TestProbeFLAC is a test.
func TestProbeFLAC(t *testing.T) {
	f, size := openFixture(t, "tone.flac")
	defer f.Close()
	info, err := media.Probe(f, size)
	if err != nil {
		t.Fatal(err)
	}
	if info.Tags.Title != "Test Tone" {
		t.Fatalf("title %q", info.Tags.Title)
	}
	if info.Duration < 900*time.Millisecond {
		t.Fatalf("duration %v", info.Duration)
	}
}
