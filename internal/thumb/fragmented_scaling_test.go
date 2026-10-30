package thumb

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

type countingReaderAt struct {
	r     *bytes.Reader
	calls atomic.Int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	c.calls.Add(1)
	return c.r.ReadAt(p, off)
}

// TestFragmentedSeekScaling checks that with mfra, doubling the file length
// does not double the reads needed to locate a keyframe (tfra binary search),
// and reports the linear moof-header hop cost without mfra.
func TestFragmentedSeekScaling(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	reads := map[string]int64{}
	for _, flags := range []string{"frag_keyframe+empty_moov", "frag_keyframe+empty_moov+skip_trailer"} {
		for _, d := range []int{120, 240} {
			src := filepath.Join(dir, fmt.Sprintf("%d.mp4", d))
			if out, err := exec.Command(ffmpeg, "-y", "-v", "error", "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=s=160x90:r=10:d=%d", d),
				"-c:v", "libx264", "-preset", "ultrafast", "-g", "10", "-movflags", flags, src).CombinedOutput(); err != nil {
				t.Skipf("encode: %v %s", err, out)
			}
			data, _ := os.ReadFile(src)
			c := &countingReaderAt{r: bytes.NewReader(data)}
			start := time.Now()
			if _, ok := isobmff.H264SyncTimeSec(c, int64(len(data)), 0, 50); !ok {
				t.Fatalf("%s %ds: no keyframe", flags, d)
			}
			el := time.Since(start)
			reads[fmt.Sprint(flags, d)] = c.calls.Load()
			t.Logf("%-40s %4ds %4d fragments: %5d ReadAt calls, %v", flags, d, d, c.calls.Load(), el)
		}
	}
	a, b := reads["frag_keyframe+empty_moov120"], reads["frag_keyframe+empty_moov240"]
	if b >= 2*a {
		t.Fatalf("mfra seek reads grew %d -> %d for 2x length", a, b)
	}
}
