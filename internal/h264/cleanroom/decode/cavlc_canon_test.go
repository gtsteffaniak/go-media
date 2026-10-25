package decode

import "testing"

// TestCanonTablesWellFormed checks the canonical CAVLC codeword tables are
// prefix-free (no codeword is a prefix of another within a table) and complete
// (coeff_token covers every (TrailingOnes, TotalCoeff) the standard defines).
func TestCanonTablesWellFormed(t *testing.T) {
	isPrefixFree := func(codes [][2]int) bool { // each entry {length, code}
		for i := range codes {
			for j := range codes {
				if i == j {
					continue
				}
				li, ci := codes[i][0], codes[i][1]
				lj, cj := codes[j][0], codes[j][1]
				if li <= lj && cj>>(lj-li) == ci {
					return false // code i is a prefix of code j
				}
			}
		}
		return true
	}

	ct := map[string][]ctEntry{"NC0": coeffTokenNC0, "NC2": coeffTokenNC2, "NC4": coeffTokenNC4, "ChromaDC": coeffTokenChromaDC}
	for name, ents := range ct {
		var codes [][2]int
		seen := map[[2]int8]bool{}
		for _, e := range ents {
			codes = append(codes, [2]int{int(e.length), int(e.code)})
			seen[[2]int8{e.t0, e.tc}] = true
		}
		if !isPrefixFree(codes) {
			t.Errorf("coeff_token %s is not prefix-free", name)
		}
		// TotalCoeff 0..16, TrailingOnes 0..min(TotalCoeff,3); ChromaDC caps at TC 4.
		maxTC := 16
		if name == "ChromaDC" {
			maxTC = 4
		}
		for tc := 0; tc <= maxTC; tc++ {
			for t0 := 0; t0 <= tc && t0 <= 3; t0++ {
				if !seen[[2]int8{int8(t0), int8(tc)}] {
					t.Errorf("coeff_token %s missing (T1=%d, TC=%d)", name, t0, tc)
				}
			}
		}
	}

	for tc, ents := range totalZerosTab {
		if tc == 0 {
			continue
		}
		var codes [][2]int
		for _, e := range ents {
			codes = append(codes, [2]int{int(e.length), int(e.code)})
		}
		if !isPrefixFree(codes) {
			t.Errorf("total_zeros[%d] is not prefix-free", tc)
		}
	}
}
