package isobmff

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"sync"
	"testing"
)

func TestNormalizeSeekPercentNonFinite(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"NaN defaults to 10", math.NaN(), 10},
		{"-Inf defaults to 10", math.Inf(-1), 10},
		{"+Inf clamps to 100", math.Inf(1), 100},
		{"zero defaults to 10", 0, 10},
		{"over 100 clamps", 150, 100},
		{"in range unchanged", 42.5, 42.5},
	}
	for _, c := range cases {
		if got := normalizeSeekPercent(c.in); got != c.want {
			t.Errorf("%s: normalizeSeekPercent(%v) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

// eofAtEndReaderAt follows the io.ReaderAt contract loosely: a read that
// reaches the end of the data returns the full count together with io.EOF.
type eofAtEndReaderAt struct{ b []byte }

func (e eofAtEndReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off > int64(len(e.b)) {
		return 0, io.EOF
	}
	n := copy(p, e.b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	if off+int64(n) == int64(len(e.b)) {
		return n, io.EOF
	}
	return n, nil
}

// TestReadersReturningEOFWithData checks that a full read which also reports
// io.EOF is accepted, including tables at the end of the file.
func TestReadersReturningEOFWithData(t *testing.T) {
	layouts := []synthMP4Opts{
		{Samples: 1000, GOP: 30, SamplesPerChunk: 7, MoovAfterMdat: true},
		{Samples: 1000, GOP: 30, SamplesPerChunk: 7, Co64: true, MoovAfterMdat: true},
		{Samples: 1000, GOP: 25, SamplesPerChunk: 10},
	}
	for _, o := range layouts {
		data := buildSynthMP4(o)
		want := bytes.NewReader(data)
		got := eofAtEndReaderAt{b: data}
		size := int64(len(data))
		for _, pct := range []float64{0, 10, 50, 99, 100} {
			wantP, okW := H264SyncSample(want, size, 0, pct)
			gotP, okG := H264SyncSample(got, size, 0, pct)
			if okW != okG || !bytes.Equal(wantP, gotP) {
				t.Errorf("%+v pct %v: EOF-returning reader gave ok=%v idx=%d, want ok=%v idx=%d",
					o, pct, okG, synthSampleIndex(gotP), okW, synthSampleIndex(wantP))
			}
		}
	}
}

// nestedMdia builds a trak payload with depth mdia boxes around an stbl.
func nestedMdia(depth int) []byte {
	inner := synthBox("stbl")
	for i := 0; i < depth; i++ {
		inner = synthBox("mdia", inner)
	}
	return inner
}

func TestDeepTrakNestingUnsupported(t *testing.T) {
	// A valid trak needs mdia > minf > stbl. Six nested mdia boxes exceed
	// maxTrakContainerDepth and must be rejected instead of recursing further.
	data := synthBox("moov", synthBox("trak", nestedMdia(6)))
	if _, err := openMovieIndex(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected error for deeply nested trak")
	}
}

func TestSampleCountOverLimitUnsupported(t *testing.T) {
	// A fixed sample size of 1 with a count above maxSamples would otherwise
	// pass the file-size check and drive large per-sample allocations.
	stsz := synthBox("stsz", u32s(0, 1, maxSamples+1))
	stbl := synthBox("stbl", stsz)
	data := synthBox("moov", synthBox("trak", synthBox("mdia", synthBox("minf", stbl))))
	if _, err := openMovieIndex(bytes.NewReader(data), int64(len(data))); err == nil {
		t.Fatal("expected error for stsz sample count over limit")
	}
}

// TestConcurrentIndexLookups exercises one cached index from many goroutines.
// Run with -race to catch shared scratch state.
func TestConcurrentIndexLookups(t *testing.T) {
	data := buildSynthMP4(synthMP4Opts{Samples: 5000, GOP: 30, SamplesPerChunk: 7, Co64: true})
	r := bytes.NewReader(data)
	idx, err := openMovieIndex(r, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	tr, err := idx.track(0)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 16
	const iters = 200
	errc := make(chan error, goroutines)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				pct := float64((g*iters+i)%100 + 1)
				k, err := tr.keyframeAt(pct)
				if err != nil {
					errc <- err
					return
				}
				if k%30 != 0 {
					errc <- fmt.Errorf("keyframe %d not a sync sample", k)
					return
				}
				off, sz, err := tr.st.locate(k)
				if err != nil {
					errc <- err
					return
				}
				if got := synthSampleIndex(data[off : off+sz]); got != k {
					errc <- fmt.Errorf("locate(%d) read sample %d", k, got)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Error(err)
	}
}
