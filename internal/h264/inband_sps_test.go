package h264

import (
	"bytes"
	"testing"
)

// TestInBandSPSLengthSize checks that the NAL walk honours avcC's
// lengthSizeMinusOne: with 2-byte prefixes a 4-byte read would mis-hop and
// could report a payload byte as an SPS.
func TestInBandSPSLengthSize(t *testing.T) {
	sps := []byte{0x67, 0x42, 0x00, 0x1e}
	// 2-byte lengths: an IDR NAL whose payload contains 0x67-looking bytes,
	// then the real SPS.
	idr := []byte{0x65, 0x00, 0x00, 0x00, 0x05, 0x07, 0x07, 0x07, 0x07, 0x07}
	var s2 []byte
	s2 = append(s2, 0x00, byte(len(idr)))
	s2 = append(s2, idr...)
	s2 = append(s2, 0x00, byte(len(sps)))
	s2 = append(s2, sps...)
	if got := inBandSPS(s2, 2); !bytes.Equal(got, sps) {
		t.Fatalf("lenSize 2: got %x, want %x", got, sps)
	}
	// 1-byte lengths.
	s1 := append([]byte{byte(len(idr))}, idr...)
	s1 = append(s1, byte(len(sps)))
	s1 = append(s1, sps...)
	if got := inBandSPS(s1, 1); !bytes.Equal(got, sps) {
		t.Fatalf("lenSize 1: got %x, want %x", got, sps)
	}
	// 4-byte lengths (default and avc3 without avcC).
	s4 := append([]byte{0, 0, 0, byte(len(sps))}, sps...)
	if got := inBandSPS(s4, 4); !bytes.Equal(got, sps) {
		t.Fatalf("lenSize 4: got %x, want %x", got, sps)
	}
	if got := inBandSPS(s4, 0); !bytes.Equal(got, sps) {
		t.Fatalf("invalid lenSize falls back to 4: got %x", got)
	}
	if got := inBandSPS(s2[:len(s2)-1], 2); got != nil {
		t.Fatalf("truncated sample: got %x, want nil", got)
	}
}
