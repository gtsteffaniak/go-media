package kernels

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

func TestRGBAccumRow420SIMDMatchesPortable(t *testing.T) {
	seed := uint32(9)
	rnd := func() byte { seed = seed*1664525 + 1013904223; return byte(seed >> 24) }
	for _, cs := range []yuv.ColorSpace{yuv.BT601, yuv.BT709, yuv.BT2020} {
		for _, rng := range []yuv.Range{yuv.LimitedRange, yuv.FullRange} {
			lut := yuv.NewRGBLUT(cs, rng)
			for n := 1; n < 200; n += 7 {
				y := make([]byte, n)
				cb, cr := make([]byte, (n+1)/2), make([]byte, (n+1)/2)
				for i := range y {
					y[i] = rnd()
				}
				for i := range cb {
					cb[i], cr[i] = rnd(), rnd()
				}
				got := make([]int32, 3*n)
				want := make([]int32, 3*n)
				for i := range got {
					got[i] = int32(rnd())
					want[i] = got[i]
				}
				RGBAccumRow420(got[:n], got[n:2*n], got[2*n:], y, cb, cr, lut)
				rgbAccumRow420Portable(want[:n], want[n:2*n], want[2*n:], y, cb, cr, lut)
				for i := range got {
					if got[i] != want[i] {
						t.Fatalf("cs=%v rng=%v n=%d idx %d: got %d want %d", cs, rng, n, i, got[i], want[i])
					}
				}
			}
		}
	}
}
