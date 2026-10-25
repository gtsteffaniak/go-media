package decode

import (
	"strings"
	"testing"
)

// TestDecodeMBNxNDC reconstructs an I_NxN (intra 4x4) macroblock where all 16
// blocks use DC mode without residual and without neighbors → whole plane = 128.
// It checks I_NxN routing, parsing of the 16 modes, predMode derivation, me(CBP),
// 4x4 prediction and chroma.
func TestDecodeMBNxNDC(t *testing.T) {
	sps, err := ParseSPS(encodeSPS(66, 30, 0, 0, 0, false))
	if err != nil {
		t.Fatalf("ParseSPS: %v", err)
	}
	pps := testPPS()
	frame := NewFrame(sps)
	h := &SliceHeader{FirstMB: 0, Type: SliceI, SliceQP: 26}

	// mb_type=0 "1" (I_NxN) | 16×prev_intra4x4_pred_mode_flag=1 "1" (mode=predMode=DC)
	//   | chroma_pred_mode=0 "1" | coded_block_pattern: codeNum=3 "00100" → CBP=0
	//   | rbsp_stop_one_bit "1"
	bits := "1" + strings.Repeat("1", 16) + "1" + "00100" + "1"
	sd := newSliceDecoder(bitsReader(bits), h, sps, pps, frame)
	if err := sd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	for i := 0; i < len(frame.Y); i++ {
		if frame.Y[i] != 128 {
			t.Fatalf("Y[%d]=%d, want 128", i, frame.Y[i])
		}
	}
	for i := 0; i < len(frame.Cb); i++ {
		if frame.Cb[i] != 128 || frame.Cr[i] != 128 {
			t.Fatalf("chroma[%d]=%d/%d, want 128/128", i, frame.Cb[i], frame.Cr[i])
		}
	}
	// All 16 modes must be written as DC(2).
	for i := 0; i < 16; i++ {
		gx, gy := luma4x4BlockX[i], luma4x4BlockY[i]
		if sd.i4mode[gy*sd.w4+gx] != 2 {
			t.Errorf("block %d: mode=%d, want 2", i, sd.i4mode[gy*sd.w4+gx])
		}
	}
}
