package h264

import "testing"

func TestInverseTransform4x4ResidualParity(t *testing.T) {
	cases := [][16]int32{
		{96, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, -1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	for _, in := range cases {
		want := inverseTransform4x4ResidualPortable(in)
		got := InverseTransform4x4Residual(in)
		for i := 0; i < 16; i++ {
			if got[i] != want[i] {
				t.Fatalf("coeff %d: got %d want %d", i, got[i], want[i])
			}
		}
	}
}
