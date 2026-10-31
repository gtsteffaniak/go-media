package vc1

import (
	"math/rand"
	"testing"
)

func TestVLCBrute(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for s := 0; s < 8; s++ {
		for it := 0; it < 20000; it++ {
			buf := make([]byte, 8)
			rng.Read(buf)
			r := newBitReader(buf)
			got, ok := r.vlcAC(s)
			gp := r.pos
			want, wp := -1, 0
			for _, e := range acVLCTables[s] {
				r2 := newBitReader(buf)
				if r2.peek(int(e.bits)) == e.code {
					want = e.idx
					wp = int(e.bits)
					break
				}
			}
			if want >= 0 && (!ok || got != want || gp != wp) {
				t.Fatalf("set %d buf %x got %d/%d want %d/%d", s, buf, got, gp, want, wp)
			}
		}
	}
	for tb := 0; tb < 2; tb++ {
		for ch := 0; ch < 2; ch++ {
			ents := [][]dcEntry{dcLuma0[:], dcChroma0[:], dcLuma1[:], dcChroma1[:]}[tb*2+ch]
			for it := 0; it < 20000; it++ {
				buf := make([]byte, 8)
				rng.Read(buf)
				r := newBitReader(buf)
				got, ok := r.vlcDC(ch == 1, tb)
				gp := r.pos
				want, wp := -1, 0
				for _, e := range ents {
					r2 := newBitReader(buf)
					if r2.peek(int(e.bits)) == e.code {
						want = int(e.val)
						wp = int(e.bits)
						break
					}
				}
				if want >= 0 && (!ok || int(got) != want || gp != wp) {
					t.Fatalf("dc %d %d buf %x got %d/%d want %d/%d", tb, ch, buf, got, gp, want, wp)
				}
			}
		}
	}
}
