package bitstream

import "testing"

// bitsToBytes packs a string of '0'/'1' into bytes MSB-first (as in H.264).
func bitsToBytes(bits string) []byte {
	out := make([]byte, (len(bits)+7)/8)
	for i, c := range bits {
		if c == '1' {
			out[i>>3] |= 1 << uint(7-(i&7))
		}
	}
	return out
}

func TestReadBitsAndFlag(t *testing.T) {
	r := NewReader(bitsToBytes("10110100"))
	if got := r.U(3); got != 0b101 {
		t.Fatalf("U(3) = %b, want 101", got)
	}
	if got := r.U(2); got != 0b10 {
		t.Fatalf("U(2) = %b, want 10", got)
	}
	if !r.Flag() {
		t.Fatal("Flag() = false, want true")
	}
	if r.Err() != nil {
		t.Fatalf("unexpected error: %v", r.Err())
	}
}

func TestReadUE(t *testing.T) {
	// ue(v): "1"→0, "010"→1, "011"→2, "00100"→3, "00111"→6, "0001000"→7
	cases := []struct {
		bits string
		want uint32
	}{
		{"1", 0},
		{"010", 1},
		{"011", 2},
		{"00100", 3},
		{"00101", 4},
		{"00111", 6},
		{"0001000", 7},
		{"000010000", 15},
	}
	for _, c := range cases {
		r := NewReader(bitsToBytes(c.bits))
		if got := r.UE(); got != c.want || r.Err() != nil {
			t.Errorf("UE(%q) = %d (err %v), want %d", c.bits, got, r.Err(), c.want)
		}
	}
}

func TestReadSE(t *testing.T) {
	// se(v): codeNum 0→0, 1→+1, 2→−1, 3→+2, 4→−2
	cases := []struct {
		bits string
		want int32
	}{
		{"1", 0},
		{"010", 1},
		{"011", -1},
		{"00100", 2},
		{"00101", -2},
		{"00110", 3},
	}
	for _, c := range cases {
		r := NewReader(bitsToBytes(c.bits))
		if got := r.SE(); got != c.want || r.Err() != nil {
			t.Errorf("SE(%q) = %d (err %v), want %d", c.bits, got, r.Err(), c.want)
		}
	}
}

func TestEOFSticky(t *testing.T) {
	r := NewReader([]byte{0xff})
	r.U(8)     // read everything
	_ = r.U(1) // past the end
	if r.Err() != ErrEOF {
		t.Fatalf("Err = %v, want ErrEOF", r.Err())
	}
	// after an error, reads return 0 and do not panic
	if r.UE() != 0 || r.SE() != 0 {
		t.Fatal("after EOF reads must yield 0")
	}
}
