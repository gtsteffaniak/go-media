package decode

import "testing"

// FuzzParseSPS exercises SPS RBSP parsing on arbitrary input.
func FuzzParseSPS(f *testing.F) {
	f.Add(BaselineSPSRBSP(66, 11, 10, 8, 0, false))
	f.Add(BaselineSPSRBSP(100, 31, 79, 44, 0, true))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseSPS(data)
	})
}
