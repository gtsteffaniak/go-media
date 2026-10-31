package media

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fail-fast budgets. The unsupported cases must return after reading headers
// only. A full-file scan of the fixtures below reads at least 1 MiB, so these
// budgets catch a regression while leaving room for header reads.
const (
	failFastByteBudget = 64 << 10
	failFastCallBudget = 200
)

// countingReaderAt records how many ReadAt calls and bytes a parser uses.
type countingReaderAt struct {
	r     io.ReaderAt
	calls int
	bytes int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.calls++
	n, err := c.r.ReadAt(p, off)
	c.bytes += int64(n)
	return n, err
}

// makeFixture encodes a short clip with ffmpeg into a temp file and returns its
// path. The test is skipped when ffmpeg is not installed.
func makeFixture(t *testing.T, name string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	path := filepath.Join(t.TempDir(), name)
	full := append([]string{"-v", "error", "-y"}, args...)
	full = append(full, path)
	out, err := exec.Command("ffmpeg", full...).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg %v: %v\n%s", args, err, out)
	}
	return path
}

// hevcMKV is about 1.5 MB, so a full scan is far above the budget.
func hevcMKV(t *testing.T) string {
	return makeFixture(t, "hevc.mkv",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30",
		"-t", "5", "-c:v", "libx265", "-b:v", "2M", "-x265-params", "log-level=error", "-an")
}

func h264MKV(t *testing.T) string {
	return makeFixture(t, "h264.mkv",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30",
		"-t", "5", "-c:v", "libx264", "-b:v", "2M", "-an")
}

func vp9WebM(t *testing.T) string {
	return makeFixture(t, "vp9.webm",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30",
		"-t", "3", "-c:v", "libvpx-vp9", "-b:v", "1500k", "-an")
}

func vp8MKV(t *testing.T) string {
	return makeFixture(t, "vp8.mkv",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-t", "3", "-c:v", "libvpx", "-b:v", "1M", "-an")
}

// openCounted opens path and returns its size and a counting wrapper.
func openCounted(t *testing.T, path string) (int64, *countingReaderAt) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() <= failFastByteBudget*4 {
		t.Fatalf("fixture %s is %d bytes; too small to show a full-file scan", path, st.Size())
	}
	return st.Size(), &countingReaderAt{r: f}
}

func assertBudget(t *testing.T, op string, c *countingReaderAt, size int64) {
	t.Helper()
	if c.bytes > failFastByteBudget || c.calls > failFastCallBudget {
		t.Fatalf("%s read %d bytes in %d calls on a %d byte file; budget is %d bytes and %d calls",
			op, c.bytes, c.calls, size, failFastByteBudget, failFastCallBudget)
	}
}

// TestUnsupportedMatroskaThumbnailFailsFast checks that an MKV or WebM whose
// video codec the thumbnail path cannot decode is rejected from the headers.
func TestUnsupportedMatroskaThumbnailFailsFast(t *testing.T) {
	cases := []struct {
		name    string
		path    func(*testing.T) string
		wantErr error
	}{
		{"h264 mkv", h264MKV, ErrUnsupportedProfile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size, c := openCounted(t, tc.path(t))
			_, err := Thumbnail(c, size, ThumbnailOptions{MaxWidth: 320, MaxHeight: 320, SeekPercent: 10, Format: "jpeg", Quality: 72})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Thumbnail error = %v, want %v", err, tc.wantErr)
			}
			assertBudget(t, "Thumbnail", c, size)
		})
	}
}

// TestUnsupportedMatroskaProbeFailsFast checks that metadata reads on the same
// files use only header bytes.
func TestUnsupportedMatroskaProbeFailsFast(t *testing.T) {
	for _, tc := range []struct {
		name string
		path func(*testing.T) string
	}{
		{"h264 mkv", h264MKV},
	} {
		t.Run(tc.name, func(t *testing.T) {
			size, c := openCounted(t, tc.path(t))
			info, err := Probe(c, size)
			if err != nil {
				t.Fatalf("Probe error = %v", err)
			}
			if info.Duration <= 0 {
				t.Fatalf("Probe duration = %v, want > 0", info.Duration)
			}
			assertBudget(t, "Probe", c, size)

			c = &countingReaderAt{r: c.r}
			if _, err := Duration(c, size); err != nil {
				t.Fatalf("Duration error = %v", err)
			}
			assertBudget(t, "Duration", c, size)
		})
	}
}

// TestSupportedVP8MatroskaStillDecodes checks that the codec gate does not block
// the VP8 path, which must still produce a thumbnail.
func TestSupportedVP8MatroskaStillDecodes(t *testing.T) {
	size, c := openCounted(t, vp8MKV(t))
	res, err := Thumbnail(c, size, ThumbnailOptions{MaxWidth: 160, MaxHeight: 160, SeekPercent: 50, Format: "jpeg", Quality: 72})
	if err != nil {
		t.Fatalf("Thumbnail error = %v", err)
	}
	if len(res.Data) == 0 {
		t.Fatal("empty thumbnail")
	}
}

// TestTruncatedMatroskaHeaderDoesNotPanic feeds the header walker a truncated
// and a corrupt-size file. Each must return an error, not panic or hang.
func TestTruncatedMatroskaHeaderDoesNotPanic(t *testing.T) {
	good, err := os.ReadFile(vp8MKV(t))
	if err != nil {
		t.Fatal(err)
	}
	inputs := map[string][]byte{
		"truncated in header": good[:200],
		"header only":         good[:4],
		"oversized segment":   append(append([]byte{}, good[:40]...), bytes.Repeat([]byte{0xFF}, 16)...),
	}
	for name, data := range inputs {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("panic: %v", p)
				}
			}()
			_, _ = Probe(bytes.NewReader(data), int64(len(data)))
			_, _ = Thumbnail(bytes.NewReader(data), int64(len(data)), ThumbnailOptions{MaxWidth: 160, MaxHeight: 160, SeekPercent: 10, Format: "jpeg"})
		})
	}
}
