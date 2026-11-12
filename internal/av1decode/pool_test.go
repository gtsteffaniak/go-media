package av1decode

import (
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// TestDecodeSampleCachedConcurrentMixedSizes decodes temporal units of very
// different sizes from many goroutines at once. Run with -race.
func TestDecodeSampleCachedConcurrentMixedSizes(t *testing.T) {
	small := readAV1(t, "tu_small.obu")
	large := readAV1(t, "tu_large.obu")

	workers := 4 * runtime.GOMAXPROCS(0)
	iters := 12
	if testing.Short() {
		iters = 4
	}
	var wg sync.WaitGroup
	errc := make(chan error, workers*iters)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				s := small
				if (w+i)%2 == 1 {
					s = large
				}
				th, err := DecodeSampleCached(nil, s, FrameLimits{})
				if err != nil {
					errc <- err
					return
				}
				if b := th.YCbCr.Bounds(); b.Dx() != 128 || b.Dy() != 72 {
					t.Errorf("bounds %v", b)
				}
			}
		}(w)
	}
	wg.Wait()
	close(errc)
	for err := range errc {
		t.Fatal(err)
	}
}

// TestDecoderReuse checks that one Decoder decodes repeatedly and that a
// closed Decoder returns an error instead of panicking.
func TestDecoderReuse(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	d := NewDecoder()
	for i := 0; i < 3; i++ {
		if _, err := d.DecodeSample(nil, sample); err != nil {
			t.Fatal(err)
		}
	}
	d.Close()
	d.Close()
	if _, err := d.DecodeSample(nil, sample); err == nil {
		t.Fatal("closed decoder decoded")
	}
}

// TestFrameSizeLimit lowers the limit below the fixture's 128×72 and checks the
// frame is rejected as an unsupported profile, before or after decode.
func TestFrameSizeLimit(t *testing.T) {
	sample := readAV1(t, "tu_small.obu")
	oldL, oldS := MaxFrameLongSide, MaxFrameShortSide
	defer func() { MaxFrameLongSide, MaxFrameShortSide = oldL, oldS }()
	for _, lim := range [][2]int{{64, 36}, {100, 100}} { // area check, per-side check
		MaxFrameLongSide, MaxFrameShortSide = lim[0], lim[1]
		_, err := DecodeSampleCached(nil, sample, FrameLimits{})
		var pe *errs.ProfileError
		if !errors.As(err, &pe) {
			t.Fatalf("limit %v: want profile error, got %v", lim, err)
		}
	}
}
