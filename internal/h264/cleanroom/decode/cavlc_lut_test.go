package decode

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

func TestCTLUTCoversAllCodewords(t *testing.T) {
	tables := map[string][1 << cavlcPeekBits]ctSlot{
		"NC0":      ctLUTNC0,
		"NC2":      ctLUTNC2,
		"NC4":      ctLUTNC4,
		"ChromaDC": ctLUTChromaDC,
	}
	for name, lut := range tables {
		var entries []ctEntry
		switch name {
		case "NC0":
			entries = coeffTokenNC0
		case "NC2":
			entries = coeffTokenNC2
		case "NC4":
			entries = coeffTokenNC4
		default:
			entries = coeffTokenChromaDC
		}
		for _, e := range entries {
			L := int(e.length)
			C := int(e.code)
			shift := cavlcPeekBits - L
			idx := C<<shift
			got := lut[idx]
			if got.bits != uint8(L) || got.t0 != e.t0 || got.tc != e.tc {
				t.Fatalf("%s codeword L=%d C=%d: lut[%d]={%d,%d,%d} want {%d,%d,%d}",
					name, L, C, idx, got.bits, got.t0, got.tc, L, e.t0, e.tc)
			}
		}
	}
}

func TestVLCLUTMatchesBitByBit(t *testing.T) {
	for tc, ents := range totalZerosTab {
		if tc == 0 {
			continue
		}
		for _, e := range ents {
			bits := ""
			for i := int(e.length) - 1; i >= 0; i-- {
				if (e.code>>uint(i))&1 == 1 {
					bits += "1"
				} else {
					bits += "0"
				}
			}
			r := bitsReader(bits + "0000000000000000")
			want := int(e.val)
			got := decodeVLC(r, &tzLUTs[tc])
			if got != want || r.BitPos() != int(e.length) {
				t.Fatalf("total_zeros tc=%d bits=%q: got %d used %d want %d used %d",
					tc, bits, got, r.BitPos(), want, e.length)
			}
		}
	}
}

// Ensure LUT decode matches a bit-by-bit walk for coeff_token (regression guard).
func TestCoeffTokenLUTMatchesWalk(t *testing.T) {
	walk := func(r *bitstream.Reader, entries []ctEntry) (int, int, int) {
		code := 0
		for length := 1; length <= cavlcPeekBits; length++ {
			code = code<<1 | int(r.Bit())
			for _, e := range entries {
				if int(e.length) == length && int(e.code) == code {
					return int(e.t0), int(e.tc), length
				}
			}
		}
		return 0, 0, r.BitPos()
	}
	for _, tab := range []struct {
		name string
		nc   int
		ents []ctEntry
		lut  *[1 << cavlcPeekBits]ctSlot
	}{
		{"NC0", 0, coeffTokenNC0, &ctLUTNC0},
		{"NC2", 2, coeffTokenNC2, &ctLUTNC2},
		{"ChromaDC", -1, coeffTokenChromaDC, &ctLUTChromaDC},
	} {
		for _, e := range tab.ents {
			bits := ""
			for i := int(e.length) - 1; i >= 0; i-- {
				if (e.code>>uint(i))&1 == 1 {
					bits += "1"
				} else {
					bits += "0"
				}
			}
			r := bitsReader(bits + "0000000000000000")
			t0w, tcw, used := walk(r, tab.ents)
			r2 := bitsReader(bits + "0000000000000000")
			t0l, tcl := decodeCoeffTokenLUT(r2, tab.lut)
			if t0w != t0l || tcw != tcl || r2.BitPos() != used {
				t.Fatalf("%s %q walk %d/%d@%d lut %d/%d@%d",
					tab.name, bits, t0w, tcw, used, t0l, tcl, r2.BitPos())
			}
		}
	}
}
