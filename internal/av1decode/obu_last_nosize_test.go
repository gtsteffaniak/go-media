package av1decode

import (
	"bytes"
	"testing"

	"github.com/gen2brain/gav1d/av1"
)

func TestNextOBULastWithoutSizeField(t *testing.T) {
	payload := []byte("obu-payload")
	// type 1 (sequence header class), no extension, no size field per AV1-ISOBMFF §2.4.
	hdr := byte(1 << 3)
	obu := append([]byte{hdr}, payload...)
	got, rest, ok := nextOBU(obu)
	if !ok || len(rest) != 0 || !bytes.Equal(got, obu) {
		t.Fatalf("ok=%v rest=%q got=%q", ok, rest, got)
	}
}

func TestDecodeSampleLastOBUWithoutSize(t *testing.T) {
	sample := av1.Encode(av1.EncodeConfig{Width: 64, Height: 64, BitDepth: 8, QIndex: 128, Speed: 10, Threads: 1})
	sample = stripSizeFromLastOBU(t, sample)
	th, err := DecodeSampleCached(nil, sample, FrameLimits{})
	if err != nil || th == nil || th.Bounds().Dx() != 64 {
		t.Fatalf("decode with last OBU omitting size: err=%v th=%v", err, th)
	}
}

// stripSizeFromLastOBU rebuilds a temporal unit whose final OBU omits the
// size field, as AV1-ISOBMFF allows for the last OBU in a sample.
func stripSizeFromLastOBU(t *testing.T, tu []byte) []byte {
	t.Helper()
	var parts [][]byte
	for len(tu) > 0 {
		obu, rest, ok := nextOBU(tu)
		if !ok {
			t.Fatal("invalid TU")
		}
		parts = append(parts, bytes.Clone(obu))
		tu = rest
	}
	if len(parts) == 0 {
		return tu
	}
	parts[len(parts)-1] = obuWithoutSizeField(parts[len(parts)-1])
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func obuWithoutSizeField(obu []byte) []byte {
	if len(obu) == 0 {
		return obu
	}
	hdr := 1
	if obu[0]&0x04 != 0 {
		hdr = 2
	}
	if obu[0]&0x02 == 0 {
		return obu
	}
	n := 0
	for i := 0; i < 8; i++ {
		if hdr+i >= len(obu) {
			return obu
		}
		n++
		if obu[hdr+i]&0x80 == 0 {
			break
		}
	}
	payload := obu[hdr+n:]
	out := make([]byte, 0, hdr+len(payload))
	out = append(out, obu[0]&^0x02)
	if hdr == 2 {
		out = append(out, obu[1])
	}
	return append(out, payload...)
}
