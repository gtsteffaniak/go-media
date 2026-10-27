package bitstream

import "testing"

func TestPeekBitsMSBMatchesSlow(t *testing.T) {
	slow := peekBitsMSBSlow
	data := bitsToBytes("10110100101101001111001100110011")
	for pos := 0; pos < 32; pos++ {
		for n := 1; n <= 16; n++ {
			got := peekBitsMSB(data, pos, n)
			want := slow(data, pos, n)
			if got != want {
				t.Fatalf("pos=%d n=%d got %x want %x", pos, n, got, want)
			}
		}
	}
}
