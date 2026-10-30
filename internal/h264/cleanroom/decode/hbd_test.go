package decode

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

func TestAddQPDeltaHighBitDepth(t *testing.T) {
	sd := &sliceDecoder{}
	sd.qpOff = 12 // 10-bit
	cases := []struct{ qp, d, want int }{
		{-12, -1, 51}, {51, 1, -12}, {0, -12, -12}, {10, 5, 15}, {-12, 63, 51},
	}
	for _, c := range cases {
		sd.qp = c.qp
		sd.addQPDelta(c.d)
		if sd.qp != c.want {
			t.Errorf("qp %d + %d = %d, want %d", c.qp, c.d, sd.qp, c.want)
		}
	}
	sd.qpOff = 0
	sd.qp = 0
	sd.addQPDelta(-1)
	if sd.qp != 51 {
		t.Errorf("8-bit wrap = %d", sd.qp)
	}
}

func TestChromaQPHighBitDepth(t *testing.T) {
	sd := &sliceDecoder{pps: &PPS{ChromaQPIndexOffset: -12, SecondChromaQPIndexOffset: 12}}
	sd.qpOff, sd.qpOffC = 12, 12
	if got := sd.chromaQPW(0, -12); got != -12 { // qPi clipped to -QpBdOffsetC
		t.Errorf("cb = %d", got)
	}
	if got := sd.chromaQPW(1, 51); got != 39 { // qPi clipped to 51 -> 39
		t.Errorf("cr = %d", got)
	}
	if got := sd.chromaDqQP(1, 12); got != 12+12 { // QPY 0 -> qPi 12 -> QP'c 24
		t.Errorf("QP'c = %d", got)
	}
}

func TestDequantWideNoSaturation(t *testing.T) {
	var c [16]int32
	c[0] = 2000
	out := dequant4x4Wide(c, 63) // QP'Y 63 (10-bit QPY 51)
	if out[0] <= 32767 {
		t.Fatalf("wide dequant saturated: %d", out[0])
	}
}

func TestDownconvertRounding(t *testing.T) {
	dst := make([]byte, 4)
	down(dst, []uint16{0, 1, 2, 1023}, 10)
	if dst[0] != 0 || dst[1] != 0 || dst[2] != 1 || dst[3] != 255 {
		t.Fatalf("down = %v", dst)
	}
}

// bitsFrom packs a string of '0'/'1' into MSB-first bytes.
func bitsFrom(s string) []byte {
	b := make([]byte, (len(s)+7)/8+4)
	for i, c := range s {
		if c == '1' {
			b[i/8] |= 0x80 >> uint(i%8)
		}
	}
	return b
}

func TestLevelEscapePrefix16(t *testing.T) {
	// level_prefix 16, suffixLength 0: 13-bit suffix of 5 ->
	// levelCode = (15<<0) + 5 + 15 + (1<<13) - 4096 = 4131.
	r := bitstream.NewReader(bitsFrom("0000000000101"))
	if got := levelEscape(r, 16, 0); got != 4131 || r.Err() != nil {
		t.Fatalf("levelCode = %d err=%v", got, r.Err())
	}
	r = bitstream.NewReader(bitsFrom("0000000000000"))
	if got := levelEscape(r, 16, 2); got != (15<<2)+4096 {
		t.Fatalf("suffixLength 2 levelCode = %d", got)
	}
	r = bitstream.NewReader(make([]byte, 8))
	levelEscape(r, maxLevelPrefix+1, 1)
	if r.Err() == nil {
		t.Fatal("over-long level_prefix accepted")
	}
}
