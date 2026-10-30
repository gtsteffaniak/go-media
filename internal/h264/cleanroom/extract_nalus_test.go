package cleanroom

import (
	"bytes"
	"testing"
)

// TestExtractAVCNalusLengthSize splits 1-, 2- and 4-byte length-prefixed NALs.
func TestExtractAVCNalusLengthSize(t *testing.T) {
	a, b := []byte{0x67, 1, 2}, []byte{0x65, 9}
	cases := map[int][]byte{
		1: {3, 0x67, 1, 2, 2, 0x65, 9},
		2: {0, 3, 0x67, 1, 2, 0, 2, 0x65, 9},
		4: {0, 0, 0, 3, 0x67, 1, 2, 0, 0, 0, 2, 0x65, 9},
	}
	for n, data := range cases {
		got, err := extractAVCNalus(data, n)
		if err != nil || len(got) != 2 || !bytes.Equal(got[0], a) || !bytes.Equal(got[1], b) {
			t.Fatalf("lenSize %d: got %x, %v", n, got, err)
		}
	}
	if _, err := extractAVCNalus([]byte{5, 0x65}, 1); err == nil {
		t.Fatal("expected error for length past end of data")
	}
}
