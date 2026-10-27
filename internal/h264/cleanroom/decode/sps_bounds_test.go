package decode

import (
	"strings"
	"testing"
)

func TestParseSPSRejectsBounds(t *testing.T) {
	t.Run("seq_parameter_set_id", func(t *testing.T) {
		rbsp := encodeSPSOpts(66, 30, 32, 0, 0, 0, false, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "seq_parameter_set_id") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("log2_max_frame_num", func(t *testing.T) {
		rbsp := encodeSPSOpts(66, 30, 0, 0, 0, 0, false, 13, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "log2_max_frame_num_minus4") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("log2_max_pic_order_cnt_lsb", func(t *testing.T) {
		rbsp := encodeSPSOpts(66, 30, 0, 0, 0, 0, false, 0, 0, 13, 0, 0, 0, 0, 0, 0, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "log2_max_pic_order_cnt_lsb_minus4") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("poc_cycle", func(t *testing.T) {
		rbsp := encodeSPSOpts(66, 30, 0, 0, 0, 0, false, 0, 1, 0, 256, 0, 0, 0, 0, 0, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "num_ref_frames_in_pic_order_cnt_cycle") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("chroma_format_idc", func(t *testing.T) {
		rbsp := encodeSPSOpts(100, 31, 0, 0, 0, 0, true, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "chroma_format_idc") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("bit_depth", func(t *testing.T) {
		rbsp := encodeSPSOpts(100, 31, 0, 0, 0, 0, true, 0, 0, 0, 0, 1, 7, 0, 0, 0, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "bit depth") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("mb_count_overflow", func(t *testing.T) {
		old := MaxFrameMacroblocks
		MaxFrameMacroblocks = ^uint64(0)
		defer func() { MaxFrameMacroblocks = old }()
		rbsp := encodeSPSOpts(66, 30, 0, 65535, 65535, 0, false, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil {
			t.Fatalf("ParseSPS: expected error")
		}
		msg := err.Error()
		if !strings.Contains(msg, "overflow") && !strings.Contains(msg, "out of range") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("macroblock_budget", func(t *testing.T) {
		rbsp := encodeSPS(66, 30, 399, 399, 0, false)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "macroblocks exceeds") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
	t.Run("oversized_crop", func(t *testing.T) {
		rbsp := encodeSPSOpts(66, 30, 0, 0, 0, 0, false, 0, 0, 0, 0, 0, 0, 0, 0, 16, 0, 0)
		_, err := ParseSPS(rbsp)
		if err == nil || !strings.Contains(err.Error(), "crop") {
			t.Fatalf("ParseSPS: %v", err)
		}
	})
}

func TestParsePPSRejectsID(t *testing.T) {
	w := &bitWriter{}
	w.ue(256)
	w.ue(0)
	w.flag(false)
	w.flag(false)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.flag(false)
	w.u(2, 0)
	w.se(0)
	w.se(0)
	w.se(0)
	w.flag(false)
	w.flag(false)
	w.flag(false)
	_, err := ParsePPS(w.bytes())
	if err == nil || !strings.Contains(err.Error(), "pic_parameter_set_id") {
		t.Fatalf("ParsePPS: %v", err)
	}
}

func TestNewFrameRejectsInvalidDimensions(t *testing.T) {
	s := &SPS{WidthMbs: 1, HeightMapUnits: 1, FrameMbsOnly: true, Width: -1, Height: 16}
	_, err := NewFrame(s)
	if err == nil {
		t.Fatal("expected error")
	}
}
