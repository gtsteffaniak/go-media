package bitstream

import "testing"

func TestReadUE(t *testing.T) {
	// codeword for ue(0) is a single bit 1 → 0x80
	r := New([]byte{0x80})
	v, err := r.ReadUE()
	if err != nil || v != 0 {
		t.Fatalf("ue(0): got %d err %v", v, err)
	}
}
