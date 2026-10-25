package reconstruct

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/transform"
)

func TestReconstructLuma4x4MatchesPortablePath(t *testing.T) {
	var pred [4][4]uint8
	for y := range 4 {
		for x := range 4 {
			pred[y][x] = uint8(40 + y*4 + x)
		}
	}
	var scan [16]int32
	scan[1] = 1
	scan[2] = -1
	scan[4] = 2
	residual := transform.DequantInverse4x4FromScan(scan, 26, nil)

	yAsm := make([]byte, 64)
	yRef := make([]byte, 64)
	stride := 8
	ReconstructLuma4x4(yAsm, stride, 2, 3, pred, residual)
	reconstructLuma4x4Portable(yRef, stride, 2, 3, pred, residual)
	for i := range yAsm {
		if yAsm[i] != yRef[i] {
			t.Fatalf("byte %d: asm %d ref %d", i, yAsm[i], yRef[i])
		}
	}
}
