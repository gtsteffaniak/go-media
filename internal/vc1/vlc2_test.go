package vc1

import (
	"os"
	"testing"
)

func TestDecode210Values(t *testing.T) {
	for _, c := range []struct {
		b0, b1 bool
		want   int
	}{
		{true, false, 0},
		{false, true, 1},
		{false, false, 2},
	} {
		r := newBitReader(nil)
		r.buf = []byte{0}
		r.pos = 0
		if c.b0 {
			r.buf[0] = 0x80
		} else if c.b1 {
			r.buf[0] = 0x40
		}
		got := decode210(&r)
		if got != c.want {
			t.Fatalf("decode210 bits %v,%v got %d want %d", c.b0, c.b1, got, c.want)
		}
	}
}

func TestVLC2Set5At8492(t *testing.T) {
	sample, err := os.ReadFile("testdata/frame1_360x288.bin")
	if err != nil {
		t.Fatal(err)
	}
	r := newBitReader(sample)
	r.pos = 8492
	ensureVLC2()
	sym, ok := r.getVLC2(acVLC2[5], acVLCBits, 3)
	if !ok || sym != 96 {
		t.Fatalf("sym=%d ok=%v", sym, ok)
	}
	if r.pos != 8504 {
		t.Fatalf("pos=%d want 8504", r.pos)
	}
}

func TestVLC2DCChroma8505(t *testing.T) {
	sample, _ := os.ReadFile("testdata/frame1_360x288.bin")
	r := newBitReader(sample)
	r.pos = 8505
	ensureVLC2()
	sym, ok := r.getVLC2(dcVLC2[1][1], dcVLCBits, 3)
	if !ok || sym != 3 {
		t.Fatalf("dc sym=%d ok=%v pos=%d", sym, ok, r.pos)
	}
}
