package thumb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

func mp4Box(typ string, payload []byte) []byte {
	b := make([]byte, 8, 8+len(payload))
	binary.BigEndian.PutUint32(b, uint32(8+len(payload)))
	copy(b[4:], typ)
	return append(b, payload...)
}

// TestUnsupportedReasonSynthetic checks container-level reasons that need no
// encoder: they come straight from the bounded top-level box walk.
func TestUnsupportedReasonSynthetic(t *testing.T) {
	ftyp := mp4Box("ftyp", []byte("isom\x00\x00\x02\x00isomavc1"))
	cases := map[string]struct {
		data []byte
		want string
	}{
		"no moov":   {append(append([]byte{}, ftyp...), mp4Box("mdat", make([]byte, 64))...), "mp4: no moov box"},
		"no trak":   {append(append([]byte{}, ftyp...), mp4Box("moov", mp4Box("mvhd", make([]byte, 100)))...), "mp4: moov has no trak boxes"},
		"truncated": {append(append([]byte{}, ftyp...), mp4Box("moov", nil)[:4]...), "mp4: no moov box"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Generate(bytes.NewReader(c.data), int64(len(c.data)), Options{})
			assertUnsup(t, err, c.want)
		})
	}
}

// sparseFile is a ReaderAt over data followed by zeros up to size, so a
// 65 MiB moov declaration can be tested without allocating it.
type sparseFile struct{ data []byte }

func (s sparseFile) ReadAt(p []byte, off int64) (int, error) {
	clear(p)
	if off < int64(len(s.data)) {
		copy(p, s.data[off:])
	}
	return len(p), nil
}

func TestUnsupportedReasonHugeMoov(t *testing.T) {
	b := mp4Box("ftyp", []byte("isom\x00\x00\x02\x00isomavc1"))
	h := make([]byte, 8)
	binary.BigEndian.PutUint32(h, 65<<20)
	copy(h[4:], "moov")
	b = append(b, h...)
	_, err := Generate(sparseFile{b}, int64(len(b))+65<<20, Options{})
	assertUnsup(t, err, "mp4: moov exceeds 64 MiB")
	_, err = Generate(bytes.NewReader(b), int64(len(b)), Options{})
	assertUnsup(t, err, `mp4: "moov" box at offset 24: size 68157440 invalid`)
}

func assertUnsup(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error containing %q, got nil", want)
	}
	if !errors.Is(err, errs.Unsupported) && !errors.Is(err, errs.UnsupportedProfile) {
		t.Fatalf("error %v does not match errs.Unsupported/UnsupportedProfile", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

// TestUnsupportedReasonCodecs encodes small files with ffmpeg and checks that
// each unsupported codec is named, so caller logs say why go-media fell back.
func TestUnsupportedReasonCodecs(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	src := []string{"-f", "lavfi", "-i", "testsrc2=s=96x64:r=10:d=1"}
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"vp9", []string{"-c:v", "libvpx-vp9"}, "mp4: video codec vp09 (VP9) not supported"},
		{"audio only", []string{"-f", "lavfi", "-i", "sine=d=1", "-map", "1:a", "-c:a", "aac"}, "mp4: no video track"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "v.mp4")
			args := append(append([]string{"-v", "error"}, src...), c.args...)
			if b, err := exec.Command(ffmpeg, append(args, out)...).CombinedOutput(); err != nil {
				t.Skipf("encoder unavailable: %v %s", err, b)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			_, err = Generate(bytes.NewReader(data), int64(len(data)), Options{})
			assertUnsup(t, err, c.want)
		})
	}
}
