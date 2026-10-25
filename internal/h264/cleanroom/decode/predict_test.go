package decode

import "testing"

func fill(n int, v byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = v
	}
	return b
}

func ramp(n int, start byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = start + byte(i)
	}
	return b
}

func TestIntra16x16Vertical(t *testing.T) {
	top := ramp(16, 0) // 0..15
	dst := make([]byte, 256)
	predictIntra16x16(dst, I16Vertical, top, fill(16, 99), 0, true, true)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if dst[y*16+x] != top[x] {
				t.Fatalf("Vert dst[%d,%d]=%d, want %d", x, y, dst[y*16+x], top[x])
			}
		}
	}
}

func TestIntra16x16Horizontal(t *testing.T) {
	left := ramp(16, 10)
	dst := make([]byte, 256)
	predictIntra16x16(dst, I16Horizontal, fill(16, 99), left, 0, true, true)
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if dst[y*16+x] != left[y] {
				t.Fatalf("Horiz dst[%d,%d]=%d, want %d", x, y, dst[y*16+x], left[y])
			}
		}
	}
}

func TestIntra16x16DC(t *testing.T) {
	dst := make([]byte, 256)
	// Both neighbors available, all =100 → (1600+1600+16)>>5 = 100.
	predictIntra16x16(dst, I16DC, fill(16, 100), fill(16, 100), 0, true, true)
	if dst[0] != 100 || dst[255] != 100 {
		t.Fatalf("DC both = %d/%d, want 100", dst[0], dst[255])
	}
	// No neighbors → 128.
	predictIntra16x16(dst, I16DC, fill(16, 0), fill(16, 0), 0, false, false)
	if dst[0] != 128 {
		t.Fatalf("DC none = %d, want 128", dst[0])
	}
}

func TestIntra16x16PlaneConstant(t *testing.T) {
	// All neighbors = C → H=V=0, pred = C across the whole block.
	const C = 70
	dst := make([]byte, 256)
	predictIntra16x16(dst, I16Plane, fill(16, C), fill(16, C), C, true, true)
	for i := range dst {
		if dst[i] != C {
			t.Fatalf("Plane const dst[%d]=%d, want %d", i, dst[i], byte(C))
		}
	}
}

func TestIntraChromaVertical(t *testing.T) {
	top := ramp(8, 5)
	dst := make([]byte, 64)
	predictIntraChroma8x8(dst, IChromaVertical, top, fill(8, 0), 0, true, true)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if dst[y*8+x] != top[x] {
				t.Fatalf("chroma vert [%d,%d]=%d, want %d", x, y, dst[y*8+x], top[x])
			}
		}
	}
}

func TestIntraChromaDCBlocks(t *testing.T) {
	// top[0..3]=40, top[4..7]=200, left=80. Both neighbors available.
	top := make([]byte, 8)
	for i := 0; i < 4; i++ {
		top[i] = 40
	}
	for i := 4; i < 8; i++ {
		top[i] = 200
	}
	left := fill(8, 80)
	dst := make([]byte, 64)
	predictIntraChroma8x8(dst, IChromaDC, top, left, 0, true, true)

	// Block (0,0): both → (40*4 + 80*4 + 4)>>3 = (160+320+4)>>3 = 60.
	if dst[0] != 60 {
		t.Errorf("DC block(0,0)=%d, want 60", dst[0])
	}
	// Block (4,0): prefers top → (200*4+2)>>2 = 200.
	if dst[4] != 200 {
		t.Errorf("DC block(4,0)=%d, want 200", dst[4])
	}
	// Block (0,4): prefers left → (80*4+2)>>2 = 80.
	if dst[4*8+0] != 80 {
		t.Errorf("DC block(0,4)=%d, want 80", dst[4*8+0])
	}
	// Block (4,4): both → (200*4+80*4+4)>>3 = (800+320+4)>>3 = 140.
	if dst[4*8+4] != 140 {
		t.Errorf("DC block(4,4)=%d, want 140", dst[4*8+4])
	}
}

func TestIntraChromaPlaneConstant(t *testing.T) {
	const C = 128
	dst := make([]byte, 64)
	predictIntraChroma8x8(dst, IChromaPlane, fill(8, C), fill(8, C), C, true, true)
	for i := range dst {
		if dst[i] != C {
			t.Fatalf("chroma plane const dst[%d]=%d, want %d", i, dst[i], byte(C))
		}
	}
}
