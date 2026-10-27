package yuv

import "testing"

// TestRGBLUTMatchesYCbCrToRGBCS checks the tables are bit-exact for every input.
func TestRGBLUTMatchesYCbCrToRGBCS(t *testing.T) {
	for _, cs := range []ColorSpace{BT601, BT709, BT2020} {
		for _, rng := range []Range{LimitedRange, FullRange} {
			l := NewRGBLUT(cs, rng)
			for y := 0; y < 256; y++ {
				for cb := 0; cb < 256; cb++ {
					for cr := 0; cr < 256; cr++ {
						r, g, b := YCbCrToRGBCS(y, cb, cr, cs, rng)
						yv := l.Y[y]
						if uint8(Clamp8(yv+l.CrR[cr])) != r || uint8(Clamp8(yv+l.CbG[cb]+l.CrG[cr])) != g || uint8(Clamp8(yv+l.CbB[cb])) != b {
							t.Fatalf("cs=%v rng=%v y=%d cb=%d cr=%d mismatch", cs, rng, y, cb, cr)
						}
					}
				}
			}
		}
	}
}
