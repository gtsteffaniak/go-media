package av1decode

import "testing"

func TestNextOBURejectsOverflowingSize(t *testing.T) {
	// On 32-bit, a signed int length accumulation turns negative; uint64
	// rejects this before slicing.
	malformed := []byte{0x0a, 0x80, 0x80, 0x80, 0x80, 0x08}
	if _, _, ok := nextOBU(malformed); ok {
		t.Fatal("expected invalid OBU")
	}
}

func TestNextOBUValidRoundTrip(t *testing.T) {
	payload := []byte("hello")
	// type 1 (sequence), has size, no extension: 0x0a
	obu := append([]byte{0x0a, byte(len(payload))}, payload...)
	got, rest, ok := nextOBU(obu)
	if !ok || len(rest) != 0 || len(got) != len(obu) {
		t.Fatalf("ok=%v len(got)=%d rest=%q", ok, len(got), rest)
	}
}
