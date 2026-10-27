package decode

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
)

func TestDecodeIPCMSlice(t *testing.T) {
	// 16x16 Baseline 4:2:0 frame (1x1 MB).
	sps, err := ParseSPS(encodeSPS(66, 30, 0, 0, 0, false))
	if err != nil {
		t.Fatalf("ParseSPS: %v", err)
	}
	pps := testPPS() // CAVLC, no deblocking
	frame, err := NewFrame(sps)
	if err != nil {
		t.Fatalf("NewFrame: %v", err)
	}

	rbsp := BuildIPCMSliceRBSP(sps, 0)
	h, err := DecodeSlice(rbsp, nal.TypeIDR, 3, sps, pps, frame, false)
	if err != nil {
		t.Fatalf("DecodeSlice: %v", err)
	}
	if h.SliceQP != 26 {
		t.Errorf("SliceQP = %d, want 26", h.SliceQP)
	}

	// Check that the samples landed in the planes in the correct raster order.
	for i := 0; i < 256; i++ {
		if frame.Y[i] != byte(i) {
			t.Fatalf("Y[%d] = %d, want %d", i, frame.Y[i], byte(i))
		}
	}
	for i := 0; i < 64; i++ {
		if frame.Cb[i] != byte(i) {
			t.Fatalf("Cb[%d] = %d, want %d", i, frame.Cb[i], byte(i))
		}
		if frame.Cr[i] != byte(63-i) {
			t.Fatalf("Cr[%d] = %d, want %d", i, frame.Cr[i], byte(63-i))
		}
	}

	// Visible region of the correct size.
	img := frame.YCbCr()
	if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 16 {
		t.Errorf("visible size = %dx%d, want 16x16", b.Dx(), b.Dy())
	}
}

func TestDecodeIMBTypeTable(t *testing.T) {
	// Points from the I-slice mb_type table.
	cases := []struct {
		mbType                   uint32
		kind                     MBKind
		mode, cbpChroma, cbpLuma int
	}{
		{0, MbINxN, 0, 0, 0},
		{1, MbI16x16, 0, 0, 0},   // I_16x16_0_0_0
		{13, MbI16x16, 0, 0, 15}, // I_16x16_0_0_1
		{24, MbI16x16, 3, 2, 15}, // I_16x16_3_2_1
		{25, MbIPCM, 0, 0, 0},
	}
	for _, c := range cases {
		mb, err := decodeIMBType(c.mbType)
		if err != nil {
			t.Fatalf("decodeIMBType(%d): %v", c.mbType, err)
		}
		if mb.Kind != c.kind || mb.Intra16x16PredMode != c.mode ||
			mb.CBPChroma != c.cbpChroma || mb.CBPLuma != c.cbpLuma {
			t.Errorf("mb_type=%d → %+v, did not match expected %v/%d/%d/%d",
				c.mbType, mb, c.kind, c.mode, c.cbpChroma, c.cbpLuma)
		}
	}
}
