package isobmff

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// countingReader counts ReadAt calls and bytes read.
type countingReader struct {
	r     *bytes.Reader
	calls int
	bytes int64
}

func (c *countingReader) ReadAt(p []byte, off int64) (int, error) {
	c.calls++
	n, err := c.r.ReadAt(p, off)
	c.bytes += int64(n)
	return n, err
}

// makeFragMP4 writes a fragmented MP4 (video + audio, one fragment per second)
// with ffmpeg. trailer controls whether the mfra index is written.
func makeFragMP4(t *testing.T, ffmpeg, dir string, seconds int, trailer bool) string {
	t.Helper()
	flags := "frag_keyframe+empty_moov"
	name := "frag" + strconv.Itoa(seconds)
	if !trailer {
		flags += "+skip_trailer"
		name += "_nomfra"
	}
	out := filepath.Join(dir, name+".mp4")
	d := strconv.Itoa(seconds)
	cmd := exec.Command(ffmpeg, "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=64x48:rate=30:duration="+d,
		"-f", "lavfi", "-i", "sine=d="+d,
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-c:a", "aac",
		"-movflags", flags, out)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg: %v %s", err, b)
	}
	return out
}

func ffprobeDuration(t *testing.T, path string) float64 {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Skipf("ffprobe: %v", err)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// probeCost returns the duration and the reads Duration made on the file.
func probeCost(t *testing.T, path string) (float64, *countingReader) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cr := &countingReader{r: bytes.NewReader(data)}
	d, err := Duration(cr, int64(len(data)))
	if err != nil {
		t.Fatalf("Duration(%s): %v", path, err)
	}
	return d.Duration.Seconds(), cr
}

// TestFragmentDurationDoesNotScaleWithLength checks that doubling the number
// of fragments does not double the probe's I/O, with and without mfra, and
// that the duration matches ffprobe.
func TestFragmentDurationDoesNotScaleWithLength(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	for _, trailer := range []bool{true, false} {
		var cost [2]*countingReader
		for i, secs := range []int{40, 80} {
			path := makeFragMP4(t, ffmpeg, dir, secs, trailer)
			got, cr := probeCost(t, path)
			want := ffprobeDuration(t, path)
			if math.Abs(got-want) > 0.05 {
				t.Fatalf("mfra=%v %ds: duration %.3f, ffprobe %.3f", trailer, secs, got, want)
			}
			cost[i] = cr
			t.Logf("mfra=%v %ds: %.3fs, %d reads, %d bytes", trailer, secs, got, cr.calls, cr.bytes)
		}
		if float64(cost[1].bytes) > 1.5*float64(cost[0].bytes) {
			t.Fatalf("mfra=%v: bytes read grew %d -> %d for 2x fragments", trailer, cost[0].bytes, cost[1].bytes)
		}
		if trailer && cost[1].calls > cost[0].calls+4 {
			t.Fatalf("mfra path: reads grew %d -> %d for 2x fragments", cost[0].calls, cost[1].calls)
		}
	}
}
