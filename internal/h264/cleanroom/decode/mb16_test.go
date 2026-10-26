package decode

import "testing"

// TestDecodeMB16DCOnly reconstructs a single I_16x16 macroblock (16x16 frame, 4:2:0)
// without neighbors. mb_type=3 → I_16x16_2_0_0 (Intra16x16 DC, CBPLuma=0, CBPChroma=0).
// The luma DC coefficient = 2 at QP=26 → block DC value 104 → residual 2;
// DC prediction without neighbors = 128 → reconstruction 130. Chroma without residual = 128.
func TestDecodeMB16DCOnly(t *testing.T) {
	sps, err := ParseSPS(encodeSPS(66, 30, 0, 0, 0, false))
	if err != nil {
		t.Fatalf("ParseSPS: %v", err)
	}
	pps := testPPS() // PicInitQPMinus26=0, CAVLC, no deblocking
	frame := NewFrame(sps)
	h := &SliceHeader{FirstMB: 0, Type: SliceI, SliceQP: 26}

	// slice_data of a single MB:
	//   mb_type=3 "00100" | chroma_pred_mode=0 "1" | mb_qp_delta=0 "1"
	//   DC block: coeff_token(0,1) "000101" | level "1"(+2) | total_zeros "1"(0)
	//   rbsp_stop_one_bit "1"
	bits := "00100" + "1" + "1" + "000101" + "1" + "1" + "1"
	sd := newSliceDecoder(bitsReader(bits), h, sps, pps, frame)
	if err := sd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	for i := 0; i < len(frame.Y); i++ {
		if frame.Y[i] != 130 {
			t.Fatalf("Y[%d]=%d, want 130 (128 prediction + 2 residual)", i, frame.Y[i])
		}
	}
	for i := 0; i < len(frame.Cb); i++ {
		if frame.Cb[i] != 128 || frame.Cr[i] != 128 {
			t.Fatalf("chroma[%d]=%d/%d, want 128/128", i, frame.Cb[i], frame.Cr[i])
		}
	}
}

// TestDecodeMB16 Vertical: the second MB horizontally with Vertical mode takes
// the neighbor's top row. We check that prediction actually reads the plane:
// two MBs in a row (1x... won't work — a wider frame is needed). We use a 2x1 MB
// frame with the first MB DC and the second Horizontal from the left neighbor.
func TestDecodeTwoMBsHorizontalPred(t *testing.T) {
	// 32x16 frame (2x1 MB), 4:2:0.
	sps, err := ParseSPS(encodeSPS(66, 30, 1, 0, 0, false)) // widthMbsM1=1 → 2 MB
	if err != nil {
		t.Fatalf("ParseSPS: %v", err)
	}
	pps := testPPS()
	frame := NewFrame(sps)
	h := &SliceHeader{FirstMB: 0, Type: SliceI, SliceQP: 26}

	// MB0: I_16x16 DC, no luma residual (DC coeff=0 → residual 0) → 128.
	//   DC block empty: coeff_token(0,0) for nC=0 = "1".
	mb0 := "00100" + "1" + "1" + "1"
	// MB1: I_16x16 Horizontal (mode=1) → mb_type=2 "011". No residual → takes
	//   the neighbor's left column (=128) → whole plane 128.
	mb1 := "011" + "1" + "1" + "1"
	bits := mb0 + mb1 + "1"
	sd := newSliceDecoder(bitsReader(bits), h, sps, pps, frame)
	if err := sd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !sd.mbDecoded[0] || !sd.mbDecoded[1] {
		t.Fatalf("both MBs must be decoded: %v", sd.mbDecoded)
	}
	for i := 0; i < len(frame.Y); i++ {
		if frame.Y[i] != 128 {
			t.Fatalf("Y[%d]=%d, want 128", i, frame.Y[i])
		}
	}
}
