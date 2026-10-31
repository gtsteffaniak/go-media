package hevcdecode

import "testing"

func TestSample16To8Main10(t *testing.T) {
	// h265 stores 10-bit samples in the low bits of uint16 (max 1023).
	const sample = uint16(1023)
	got := sample16To8(sample, 10)
	if got != 255 {
		t.Fatalf("sample16To8(1023, 10) = %d, want 255", got)
	}
	if sample16To8(128, 8) != 128 {
		t.Fatalf("8-bit passthrough")
	}
}
