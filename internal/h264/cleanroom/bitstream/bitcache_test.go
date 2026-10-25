package bitstream

import "testing"

func TestReadUE(t *testing.T) {
	r := New([]byte{0x80})
	v, err := r.ReadUE()
	if err != nil || v != 0 {
		t.Fatalf("ue(0): got %d err %v", v, err)
	}
}

func TestReadMultiBit(t *testing.T) {
	r := New([]byte{0x80})
	if _, err := r.Read(1); err != nil {
		t.Fatal(err)
	}
	b, err := r.Read(1)
	if err != nil || b != 0 {
		t.Fatalf("second bit: got %d err %v", b, err)
	}
}
