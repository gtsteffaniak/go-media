package kernels

import (
	"bytes"
	"testing"
)

func TestAreaDownRowYMatchesPortable(t *testing.T) {
	src := make([]byte, 16*8)
	for i := range src {
		src[i] = byte(i % 251)
	}
	for _, tc := range []struct {
		sy0, sy1, x0, x1 int
	}{
		{0, 8, 0, 16},
		{2, 5, 3, 11},
		{0, 1, 0, 1},
	} {
		dstA := make([]byte, 1)
		dstB := make([]byte, 1)
		areaDownRowYPortable(dstA, 0, src, 16, tc.sy0, tc.sy1, tc.x0, tc.x1)
		AreaDownRowY(dstB, 0, 0, src, 16, tc.sy0, tc.sy1, tc.x0, tc.x1)
		if dstA[0] != dstB[0] {
			t.Fatalf("sy=%d..%d x=%d..%d: portable=%d asm=%d", tc.sy0, tc.sy1, tc.x0, tc.x1, dstA[0], dstB[0])
		}
	}
}

func TestPackRGBARow(t *testing.T) {
	yy := []byte{16, 128, 235}
	cb := []byte{128, 64, 192}
	cr := []byte{128, 200, 64}
	dst := make([]byte, 3*4)
	packRGBARowPortable(dst, 12, 0, yy, cb, cr)
	dst2 := make([]byte, 3*4)
	PackRGBARow(dst2, 12, 0, yy, cb, cr)
	if !bytes.Equal(dst, dst2) {
		t.Fatalf("pack mismatch:\n%x\n%x", dst, dst2)
	}
}
