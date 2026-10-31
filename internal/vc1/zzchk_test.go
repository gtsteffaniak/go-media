package vc1

import "testing"

func TestZZTransposed(t *testing.T) {
	for k := 0; k < 4; k++ {
		for i := 0; i < 64; i++ {
			x := wmv1ZZ[k][i]
			if zz8[k][i] != (x>>3)|((x&7)<<3) {
				t.Fatal(k, i)
			}
		}
	}
}
