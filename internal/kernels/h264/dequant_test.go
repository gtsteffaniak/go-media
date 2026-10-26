package h264

import "testing"

func TestDequant4x4Parity(t *testing.T) {
	coef := [16]int32{1, -1, 2, 0, 3, 4, -2, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	for _, qp := range []int{6, 24, 30} {
		want := dequant4x4Portable(coef, qp)
		got := Dequant4x4(coef, qp)
		for i := 0; i < 16; i++ {
			if got[i] != want[i] {
				t.Fatalf("qp=%d idx=%d got %d want %d simd=%v", qp, i, got[i], want[i], KernelsUseSIMD)
			}
		}
	}
}
