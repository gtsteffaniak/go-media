package decode

import "testing"

// TestIntra4x4ConstantInvariant: with identical neighbors (=C) any mode yields C
// (all modes are weighted averages of neighbors with weight sum = normalization).
func TestIntra4x4ConstantInvariant(t *testing.T) {
	const C = 77
	top := fill(8, C)
	left := fill(4, C)
	dst := make([]byte, 16)
	for mode := 0; mode <= 8; mode++ {
		predictIntra4x4(dst, mode, top, left, C, true, true)
		for i := 0; i < 16; i++ {
			if dst[i] != C {
				t.Fatalf("mode %d: dst[%d]=%d, want %d", mode, i, dst[i], byte(C))
			}
		}
	}
}

func TestIntra4x4Vertical(t *testing.T) {
	top := ramp(8, 0) // 0..7
	dst := make([]byte, 16)
	predictIntra4x4(dst, I4Vertical, top, fill(4, 0), 0, true, true)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if dst[y*4+x] != top[x] {
				t.Fatalf("Vert dst[%d,%d]=%d, want %d", x, y, dst[y*4+x], top[x])
			}
		}
	}
}

func TestIntra4x4Horizontal(t *testing.T) {
	left := ramp(4, 10)
	dst := make([]byte, 16)
	predictIntra4x4(dst, I4Horizontal, fill(8, 0), left, 0, true, true)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if dst[y*4+x] != left[y] {
				t.Fatalf("Horiz dst[%d,%d]=%d, want %d", x, y, dst[y*4+x], left[y])
			}
		}
	}
}

func TestIntra4x4DC(t *testing.T) {
	dst := make([]byte, 16)
	// top=[40,40,40,40], left=[80,80,80,80] → (160+320+4)>>3 = 60.
	predictIntra4x4(dst, I4DC, fill(8, 40), fill(4, 80), 0, true, true)
	if dst[0] != 60 {
		t.Errorf("DC both=%d, want 60", dst[0])
	}
	// no neighbors → 128.
	predictIntra4x4(dst, I4DC, fill(8, 0), fill(4, 0), 0, false, false)
	if dst[0] != 128 {
		t.Errorf("DC none=%d, want 128", dst[0])
	}
}

func TestIntra4x4DiagDownLeft(t *testing.T) {
	// top = 0..7. dst[0,0] = (top[0]+2*top[1]+top[2]+2)>>2 = (0+2+2+2)>>2 = 1.
	// dst[3,3] = (top[6]+3*top[7]+2)>>2 = (6+21+2)>>2 = 29>>2 = 7.
	top := ramp(8, 0)
	dst := make([]byte, 16)
	predictIntra4x4(dst, I4DiagDownLeft, top, fill(4, 0), 0, true, true)
	if dst[0] != 1 {
		t.Errorf("DDL dst[0,0]=%d, want 1", dst[0])
	}
	if dst[3*4+3] != 7 {
		t.Errorf("DDL dst[3,3]=%d, want 7", dst[3*4+3])
	}
}
