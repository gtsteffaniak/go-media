package h264

import "testing"

func TestSIMDIDCTParity(t *testing.T) {
	cases := [][16]int32{
		{96, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, -1, 2, 0, 3, 4, -2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	for _, in := range cases {
		want := inverseTransform4x4ResidualPortable(in)
		got := InverseTransform4x4Residual(in)
		for i := 0; i < 16; i++ {
			if got[i] != want[i] {
				t.Fatalf("coeff %d: got %d want %d (simd=%v)", i, got[i], want[i], KernelsUseSIMD)
			}
		}
	}
}

func TestSIMDKernelBodiesMatchPortable(t *testing.T) {
	// SIMD bodies are exercised directly (independent of the static enables) so a
	// kernel stays correct while it is switched off pending a measured win.
	rng := uint32(1)
	next := func() int32 { rng = rng*1664525 + 1013904223; return int32(rng>>20) - 2048 }
	for iter := 0; iter < 2000; iter++ {
		var d [16]int32
		for i := range d {
			d[i] = next()
		}
		if got, want := inverseTransform4x4ResidualSIMD(d), inverseTransform4x4ResidualPortable(d); got != want {
			t.Fatalf("idct4x4 iter %d: got %v want %v", iter, got, want)
		}
	}
}
