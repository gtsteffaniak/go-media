package decode

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
)

// align appends zero bits up to the byte boundary (pcm_alignment_zero_bit).
func (w *bitWriter) align() {
	for len(w.bits)%8 != 0 {
		w.bits = append(w.bits, 0)
	}
}

// buildIPCMSlice builds the RBSP of an IDR I-slice from a single I_PCM macroblock
// with predictable samples: luma[i]=i, Cb[i]=i, Cr[i]=63-i (4:2:0, 8x8 chroma).
func buildIPCMSlice(sps *SPS) []byte {
	w := &bitWriter{}
	// slice_header (IDR, I)
	w.ue(0)                     // first_mb_in_slice
	w.ue(uint32(SliceI))        // slice_type = I
	w.ue(0)                     // pps id
	w.u(sps.Log2MaxFrameNum, 0) // frame_num
	w.ue(0)                     // idr_pic_id
	w.u(sps.Log2MaxPocLsb, 0)   // pic_order_cnt_lsb
	w.flag(false)               // no_output_of_prior_pics_flag
	w.flag(false)               // long_term_reference_flag
	w.se(0)                     // slice_qp_delta → SliceQP=26
	// macroblock_layer
	w.ue(25) // mb_type = I_PCM
	w.align()
	for i := 0; i < 256; i++ { // luma 16x16
		w.u(8, uint32(i))
	}
	for i := 0; i < 64; i++ { // Cb 8x8
		w.u(8, uint32(i))
	}
	for i := 0; i < 64; i++ { // Cr 8x8
		w.u(8, uint32(63-i))
	}
	w.u(8, 0x80) // rbsp_slice_trailing_bits: stop-one-bit + zeros
	return w.bytes()
}

func TestDecodeIPCMSlice(t *testing.T) {
	// 16x16 Baseline 4:2:0 frame (1x1 MB).
	sps, err := ParseSPS(encodeSPS(66, 30, 0, 0, 0, false))
	if err != nil {
		t.Fatalf("ParseSPS: %v", err)
	}
	pps := testPPS() // CAVLC, no deblocking
	frame := NewFrame(sps)

	rbsp := buildIPCMSlice(sps)
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
