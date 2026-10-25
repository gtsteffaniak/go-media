package bitstream

import "testing"

func TestReadUE(t *testing.T) {
	r := New([]byte{0x9a, 0x24})
	v, err := r.ReadUE()
	if err != nil || v != 0 {
		t.Fatalf("first ue: got %d err %v", v, err)
	}
	v, err = r.ReadUE()
	if err != nil || v != 1 {
		t.Fatalf("second ue: got %d err %v", v, err)
	}
}
