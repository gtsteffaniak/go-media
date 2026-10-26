package decode

import "testing"

func TestLevelScale4x4(t *testing.T) {
	// Flat matrix: LevelScale = 16·v[m][class].
	if got := levelScale4x4(0, 0, 0); got != 160 { // class 0, v=10
		t.Errorf("levelScale(0,0,0)=%d, want 160", got)
	}
	if got := levelScale4x4(0, 1, 1); got != 256 { // class 1, v=16
		t.Errorf("levelScale(0,1,1)=%d, want 256", got)
	}
	if got := levelScale4x4(0, 0, 1); got != 208 { // class 2, v=13
		t.Errorf("levelScale(0,0,1)=%d, want 208", got)
	}
}

func TestDequant4x4(t *testing.T) {
	// QP=24 → m=0, shift=4 (>=4, no rounding): d = c·160.
	var coef [16]int32
	coef[0] = 1
	d := dequant4x4(coef, 24)
	if d[0] != 160 {
		t.Errorf("dequant QP24 d[0]=%d, want 160", d[0])
	}
	// QP=6 → m=0, shift=1 (<4): d = (c·160 + 4) >> 3 = 164>>3 = 20.
	d = dequant4x4(coef, 6)
	if d[0] != 20 {
		t.Errorf("dequant QP6 d[0]=%d, want 20", d[0])
	}
}

func TestInverseTransformDCOnly(t *testing.T) {
	// DC only → uniform residual r = (d00 + 32) >> 6.
	var d [16]int32
	d[0] = 64
	r := inverseTransform4x4(d)
	for k := 0; k < 16; k++ {
		if r[k] != 1 { // (64+32)>>6 = 1
			t.Fatalf("r[%d]=%d, want 1", k, r[k])
		}
	}
	d[0] = 96
	r = inverseTransform4x4(d) // (96+32)>>6 = 2
	for k := 0; k < 16; k++ {
		if r[k] != 2 {
			t.Fatalf("r[%d]=%d, want 2", k, r[k])
		}
	}
}

func TestInverseTransformZero(t *testing.T) {
	var d [16]int32
	r := inverseTransform4x4(d)
	for k := 0; k < 16; k++ {
		if r[k] != 0 {
			t.Fatalf("zero input → r[%d]=%d, want 0", k, r[k])
		}
	}
}

func TestInverseLumaDC(t *testing.T) {
	// A single DC=1 → Hadamard yields all 1, then scaling. QP=36: m=0,shift=6,ls=160 → 160.
	var c [16]int32
	c[0] = 1
	dc := inverseLumaDC(c, 36)
	for k := 0; k < 16; k++ {
		if dc[k] != 160 {
			t.Fatalf("lumaDC[%d]=%d, want 160", k, dc[k])
		}
	}
}

func TestInverseChromaDC(t *testing.T) {
	// c=[10,0,0,0] → Hadamard yields all 10. QP=4: m=4,shift=0,ls=16·16=256.
	// scale = (10·256 << 0) >> 5 = 2560>>5 = 80.
	dc := inverseChromaDC([4]int32{10, 0, 0, 0}, 4)
	for k := 0; k < 4; k++ {
		if dc[k] != 80 {
			t.Fatalf("chromaDC[%d]=%d, want 80", k, dc[k])
		}
	}
}
