package nal

import (
	"bytes"
	"testing"
)

func TestSplitAnnexB(t *testing.T) {
	// Two NALs: the first with a 3-byte start code, the second with a 4-byte one.
	stream := []byte{
		0, 0, 1, 0x67, 0x42, 0x00, // start code(3) + SPS-like
		0, 0, 0, 1, 0x68, 0xCE, // start code(4) + PPS-like
	}
	units := SplitAnnexB(stream)
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	if !bytes.Equal(units[0], []byte{0x67, 0x42, 0x00}) {
		t.Errorf("unit[0] = % x", units[0])
	}
	if !bytes.Equal(units[1], []byte{0x68, 0xCE}) {
		t.Errorf("unit[1] = % x", units[1])
	}
}

func TestNALHeaderParse(t *testing.T) {
	// 0x67 = 0 11 00111 → forbidden=0, ref_idc=3, type=7 (SPS)
	u, ok := Parse([]byte{0x67, 0xAA, 0xBB})
	if !ok {
		t.Fatal("Parse returned ok=false")
	}
	if u.Type != TypeSPS {
		t.Errorf("Type = %v, want SPS", u.Type)
	}
	if u.RefIDC != 3 {
		t.Errorf("RefIDC = %d, want 3", u.RefIDC)
	}
}

func TestEBSPtoRBSP(t *testing.T) {
	// 00 00 03 01 → 00 00 01 (the byte 0x03 is emulation prevention, removed)
	in := []byte{0x00, 0x00, 0x03, 0x01, 0xFF, 0x00, 0x00, 0x03, 0x02}
	want := []byte{0x00, 0x00, 0x01, 0xFF, 0x00, 0x00, 0x02}
	if got := EBSPtoRBSP(in); !bytes.Equal(got, want) {
		t.Errorf("EBSPtoRBSP = % x, want % x", got, want)
	}
	// 00 00 03 04 → 0x04 > 0x03, so 0x03 is NOT an emulation byte, it stays
	in2 := []byte{0x00, 0x00, 0x03, 0x04}
	if got := EBSPtoRBSP(in2); !bytes.Equal(got, in2) {
		t.Errorf("EBSPtoRBSP(00 00 03 04) = % x, want unchanged", got)
	}
}
