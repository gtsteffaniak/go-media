package isobmff

import (
	"bytes"
	"sort"
	"sync/atomic"
	"testing"
	"time"
)

// countingReaderAt counts bytes returned by ReadAt.
type countingReaderAt struct {
	r *bytes.Reader
	n atomic.Int64
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.n.Add(int64(n))
	return n, err
}

// grabCost measures one H.264 keyframe grab at 10%: median wall time over
// runs, allocations, and bytes read from the file.
func grabCost(t testing.TB, data []byte, runs int) (time.Duration, float64, int64) {
	t.Helper()
	cr := &countingReaderAt{r: bytes.NewReader(data)}
	size := int64(len(data))
	grab := func() {
		track, ok := FirstAVCTrackIndex(cr, size)
		if !ok {
			t.Fatal("no avc track")
		}
		if _, ok := H264SyncSample(cr, size, track, 10); !ok {
			t.Fatal("no sync sample")
		}
	}
	grab()
	cr.n.Store(0)
	grab()
	bytesRead := cr.n.Load()
	allocs := testing.AllocsPerRun(1, grab)
	ds := make([]time.Duration, runs)
	for i := range ds {
		st := time.Now()
		grab()
		ds[i] = time.Since(st)
	}
	sort.Slice(ds, func(a, b int) bool { return ds[a] < ds[b] })
	return ds[runs/2], allocs, bytesRead
}

// TestKeyframeGrabScaling asserts that doubling the sample count (duration)
// does not double frame-grab cost: bytes read and allocations stay flat and
// wall time grows well under 2x.
func TestKeyframeGrabScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("scaling test generates ~10 MB of synthetic MP4")
	}
	const n = 216000 // one hour at 60 fps
	small := buildSynthMP4(synthMP4Opts{Samples: n, GOP: 120, SamplesPerChunk: 30})
	large := buildSynthMP4(synthMP4Opts{Samples: 2 * n, GOP: 120, SamplesPerChunk: 30})
	t1, a1, b1 := grabCost(t, small, 9)
	t2, a2, b2 := grabCost(t, large, 9)
	timeRatio := float64(t2) / float64(t1)
	t.Logf("N=%d: %v %.0f allocs %d B read; 2N: %v %.0f allocs %d B read; time ratio %.2fx",
		n, t1, a1, b1, t2, a2, b2, timeRatio)
	if a2 > a1*1.2+4 {
		t.Errorf("allocations scale with samples: %.0f -> %.0f", a1, a2)
	}
	if float64(b2) > float64(b1)*1.2+4096 {
		t.Errorf("bytes read scale with samples: %d -> %d", b1, b2)
	}
	if timeRatio > 1.6 && t2 > 200*time.Microsecond {
		t.Errorf("frame grab time ratio 2N/N = %.2fx, want < 1.6x", timeRatio)
	}
}
