package decode

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

// bitsReader builds a reader from a string of '0'/'1' (MSB-first, as in the stream).
func bitsReader(s string) *bitstream.Reader {
	out := make([]byte, (len(s)+7)/8)
	for i, c := range s {
		if c == '1' {
			out[i>>3] |= 1 << uint(7-(i&7))
		}
	}
	return bitstream.NewReader(out)
}

func TestCoeffTokenNC0(t *testing.T) {
	// Reference code words for 0 <= nC < 2.
	cases := []struct {
		bits         string
		t0, tc, used int
	}{
		{"1", 0, 0, 1},
		{"01", 1, 1, 2},
		{"001", 2, 2, 3},
		{"00011", 3, 3, 5},
		{"000101", 0, 1, 6},
	}
	for _, c := range cases {
		r := bitsReader(c.bits + "0000000000000000")
		t0, tc := decodeCoeffToken(r, 0)
		if t0 != c.t0 || tc != c.tc || r.BitPos() != c.used {
			t.Errorf("nC0 %q → T0=%d TC=%d used=%d, want %d/%d/%d",
				c.bits, t0, tc, r.BitPos(), c.t0, c.tc, c.used)
		}
	}
}

func TestCoeffTokenChromaDC(t *testing.T) {
	// nC == -1 (chroma DC 4:2:0): "1"→(1,1), "01"→(0,0).
	r := bitsReader("1" + "0000000")
	if t0, tc := decodeCoeffToken(r, -1); t0 != 1 || tc != 1 || r.BitPos() != 1 {
		t.Errorf("chromaDC %q → %d/%d used=%d, want 1/1/1", "1", t0, tc, r.BitPos())
	}
	r = bitsReader("01" + "000000")
	if t0, tc := decodeCoeffToken(r, -1); t0 != 0 || tc != 0 || r.BitPos() != 2 {
		t.Errorf("chromaDC %q → %d/%d used=%d, want 0/0/2", "01", t0, tc, r.BitPos())
	}
}

func TestCoeffTokenFLC(t *testing.T) {
	// nC >= 8: 6-bit FLC. code=3→(0,0); code=1→(1,1); code=4→(0,2).
	if t0, tc := decodeCoeffToken(bitsReader("000011"), 8); t0 != 0 || tc != 0 {
		t.Errorf("FLC code3 → %d/%d, want 0/0", t0, tc)
	}
	if t0, tc := decodeCoeffToken(bitsReader("000001"), 8); t0 != 1 || tc != 1 {
		t.Errorf("FLC code1 → %d/%d, want 1/1", t0, tc)
	}
	if t0, tc := decodeCoeffToken(bitsReader("000100"), 8); t0 != 0 || tc != 2 {
		t.Errorf("FLC code4 → %d/%d, want 0/2", t0, tc)
	}
}

func TestReadLevels(t *testing.T) {
	cases := []struct {
		name             string
		bits             string
		trailingOnes, tc int
		want             []int32
	}{
		{"one trailing one +", "0", 1, 1, []int32{1}},
		{"one trailing one -", "1", 1, 1, []int32{-1}},
		{"one level +2", "1", 0, 1, []int32{2}},
		{"one level -2", "010", 0, 1, []int32{-2}},
		{"trailing one + level", "01", 1, 2, []int32{1, 2}},
	}
	for _, c := range cases {
		r := bitsReader(c.bits + "00000000")
		got := readLevels(r, c.trailingOnes, c.tc)
		if len(got) != len(c.want) {
			t.Errorf("%s: length %d, want %d", c.name, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: level[%d]=%d, want %d", c.name, i, got[i], c.want[i])
			}
		}
	}
}
