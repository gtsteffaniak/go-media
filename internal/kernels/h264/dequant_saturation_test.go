package h264

import "testing"

func TestDequantSaturatesInt16Range(t *testing.T) {
	var coef [16]int32
	coef[0] = 1 << 19
	got := dequant4x4Portable(coef, 51)
	if got[0] != 32767 {
		t.Fatalf("expected saturate to 32767, got %d", got[0])
	}
	coef[0] = -(1 << 19)
	got = dequant4x4Portable(coef, 51)
	if got[0] != -32768 {
		t.Fatalf("expected saturate to -32768, got %d", got[0])
	}
}

func TestDequantTypicalUnchanged(t *testing.T) {
	var coef [16]int32
	coef[0] = 1
	got := dequant4x4Portable(coef, 24)
	if got[0] != 160 {
		t.Fatalf("unexpected dequant: %d", got[0])
	}
}
