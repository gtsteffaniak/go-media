package h264

import "testing"

func TestIDCT8x8Parity(t *testing.T) {
	d := [64]int32{}
	d[0] = 96
	want := inverseTransform8x8ResidualPortable(d)
	got := InverseTransform8x8Residual(d)
	for i := 0; i < 64; i++ {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %d want %d simd=%v", i, got[i], want[i], KernelsUseSIMD)
		}
	}
}
