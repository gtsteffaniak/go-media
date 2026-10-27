package h264

import "testing"

func TestIDCTDCSemanticsHadamardOnly(t *testing.T) {
	var plane [256]byte
	var pred [256]byte
	for i := range pred {
		pred[i] = 100
	}
	// DC-only: coeff 96 → residual 1 after rounding (same as portable IDCT path).
	IDCTDCAdd4x4(plane[:], 16, 0, 0, pred[:], 16, 0, 0, 96)
	var d [16]int32
	d[0] = 96
	want := inverseTransform4x4ResidualPortable(d)
	for yy := 0; yy < 4; yy++ {
		for xx := 0; xx < 4; xx++ {
			got := int(plane[yy*16+xx])
			exp := int(pred[yy*16+xx]) + int(want[yy*4+xx])
			if got != exp {
				t.Fatalf("(%d,%d) got %d want %d", yy, xx, got, exp)
			}
		}
	}
}

func TestIDCTAdd16ZeroBlockCopiesPred(t *testing.T) {
	var plane [256]byte
	var pred [256]byte
	pred[0] = 42
	var blocks [16][16]int32
	var nz [16]uint8
	IDCTAdd16Luma(plane[:], 16, 0, 0, pred[:], 16, &blocks, nz[:])
	if plane[0] != 42 {
		t.Fatalf("expected prediction copy, got %d", plane[0])
	}
}

func TestIDCTAdd16DCWhenNZZero(t *testing.T) {
	var plane [256]byte
	var pred [256]byte
	for i := range pred {
		pred[i] = 50
	}
	var blocks [16][16]int32
	blocks[0][0] = 96
	var nz [16]uint8 // AC absent but DC present
	IDCTAdd16Luma(plane[:], 16, 0, 0, pred[:], 16, &blocks, nz[:])
	if plane[0] == 50 {
		t.Fatal("expected DC residual added")
	}
}
