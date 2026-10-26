package decode

import "testing"

func TestIntra8x8ConstantInvariant(t *testing.T) {
	// Equal neighbors (=C) → every mode (after filtering) yields C.
	const C = 90
	ft, fl, fc := filterRef8x8(fill(16, C), fill(8, C), C, true, true, true)
	dst := make([]byte, 64)
	for mode := 0; mode <= 8; mode++ {
		predictIntra8x8(dst, mode, ft, fl, fc, true, true)
		for i := 0; i < 64; i++ {
			if dst[i] != C {
				t.Fatalf("mode %d: dst[%d]=%d, want %d", mode, i, dst[i], byte(C))
			}
		}
	}
}

func TestIntra8x8Vertical(t *testing.T) {
	// Filtered top is copied down each column.
	top := ramp(16, 0)
	ft, fl, fc := filterRef8x8(top, fill(8, 0), 0, true, false, false)
	dst := make([]byte, 64)
	predictIntra8x8(dst, I4Vertical, ft, fl, fc, false, true)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if int(dst[y*8+x]) != ft[x] {
				t.Fatalf("Vert dst[%d,%d]=%d, want %d", x, y, dst[y*8+x], ft[x])
			}
		}
	}
}

func TestFilterRef8x8(t *testing.T) {
	// top all 100, corner 100 available → ft[0]=(100+200+100... )>>2 of ramp.
	// Use constant to verify filter is identity for flat input.
	ft, fl, fc := filterRef8x8(fill(16, 100), fill(8, 50), 100, true, true, true)
	if ft[0] != 100 || ft[7] != 100 || ft[15] != 100 {
		t.Errorf("ft = %d/%d/%d, want 100", ft[0], ft[7], ft[15])
	}
	// fl[0] = (corner 100 + 2*50 + 50 + 2)>>2 = 63; fl[7] = (50 + 3*50 + 2)>>2 = 50.
	if fl[0] != 63 || fl[7] != 50 {
		t.Errorf("fl = %d/%d, want 63/50", fl[0], fl[7])
	}
	// fc = (100 + 2*100 + 50 + 2)>>2 = 352>>2 = 88.
	if fc != 88 {
		t.Errorf("fc = %d, want 88", fc)
	}
}
