package decode

import "testing"

func TestInverseTransform8x8DCOnly(t *testing.T) {
	// Only the DC coefficient → a uniform residual r = (d00 + 32) >> 6.
	var d [64]int32
	d[0] = 64
	r := inverseTransform8x8(d)
	for k := 0; k < 64; k++ {
		if r[k] != 1 { // (64+32)>>6 = 1
			t.Fatalf("r[%d]=%d, want 1", k, r[k])
		}
	}
	d[0] = 96
	r = inverseTransform8x8(d) // (96+32)>>6 = 2
	for k := 0; k < 64; k++ {
		if r[k] != 2 {
			t.Fatalf("r[%d]=%d, want 2", k, r[k])
		}
	}
}

func TestInverseTransform8x8Zero(t *testing.T) {
	var d [64]int32
	r := inverseTransform8x8(d)
	for k := 0; k < 64; k++ {
		if r[k] != 0 {
			t.Fatalf("zero input → r[%d]=%d, want 0", k, r[k])
		}
	}
}

func TestDequant8x8(t *testing.T) {
	// QP=36 → m=0, shift=6 (>=6, no rounding): d = c · 16·v8x8[0][class].
	// Position (0,0) → class 0 → v=20 → LevelScale=320.
	var coef [64]int32
	coef[0] = 1
	d := dequant8x8(coef, 36)
	if d[0] != 320 {
		t.Errorf("dequant8x8 QP36 d[0]=%d, want 320", d[0])
	}
}

func TestInverseScan8x8(t *testing.T) {
	scan := make([]int32, 64)
	scan[1] = 5 // zigzag8x8[1] = 1
	scan[2] = 7 // zigzag8x8[2] = 8
	b := inverseScan8x8(scan)
	if b[1] != 5 || b[8] != 7 {
		t.Errorf("scan→raster: b[1]=%d b[8]=%d, want 5/7", b[1], b[8])
	}
}
