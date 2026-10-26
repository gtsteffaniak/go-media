package decode

import "testing"

// bitWriter is a minimal bitstream encoder for building controlled test SPS
// (a mirror of the bitstream.Reader methods). This way the test does not depend
// on "magic" bytes from outside and exercises the reader and parser together.
type bitWriter struct{ bits []byte } // each element is 0/1

func (w *bitWriter) u(n int, v uint32) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, byte((v>>uint(i))&1))
	}
}
func (w *bitWriter) flag(b bool) {
	if b {
		w.u(1, 1)
	} else {
		w.u(1, 0)
	}
}
func bitLen(v uint32) int {
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}
func (w *bitWriter) ue(v uint32) {
	x := v + 1
	nbits := bitLen(x)
	for i := 0; i < nbits-1; i++ {
		w.bits = append(w.bits, 0)
	}
	w.u(nbits, x)
}
func (w *bitWriter) se(v int32) {
	var k uint32
	if v > 0 {
		k = uint32(2*v - 1)
	} else {
		k = uint32(-2 * v)
	}
	w.ue(k)
}
func (w *bitWriter) bytes() []byte {
	out := make([]byte, (len(w.bits)+7)/8)
	for i, b := range w.bits {
		if b == 1 {
			out[i>>3] |= 1 << uint(7-(i&7))
		}
	}
	return out
}

// encodeSPS builds an SPS RBSP in ParseSPS order. cropBottom is set separately
// (for heights not divisible by 16). high=true adds the chroma/bit_depth block.
func encodeSPS(profile, level uint8, widthMbsM1, heightMapM1, cropBottom uint32, high bool) []byte {
	w := &bitWriter{}
	w.u(8, uint32(profile))
	w.u(8, 0) // constraint flags + reserved
	w.u(8, uint32(level))
	w.ue(0) // seq_parameter_set_id
	if high {
		w.ue(1)       // chroma_format_idc = 4:2:0
		w.ue(0)       // bit_depth_luma_minus8
		w.ue(0)       // bit_depth_chroma_minus8
		w.flag(false) // qpprime_y_zero_transform_bypass_flag
		w.flag(false) // seq_scaling_matrix_present_flag
	}
	w.ue(0)       // log2_max_frame_num_minus4
	w.ue(0)       // pic_order_cnt_type = 0
	w.ue(0)       // log2_max_pic_order_cnt_lsb_minus4
	w.ue(1)       // max_num_ref_frames
	w.flag(false) // gaps_in_frame_num_value_allowed_flag
	w.ue(widthMbsM1)
	w.ue(heightMapM1)
	w.flag(true) // frame_mbs_only_flag = 1
	w.flag(true) // direct_8x8_inference_flag
	if cropBottom > 0 {
		w.flag(true) // frame_cropping_flag
		w.ue(0)      // left
		w.ue(0)      // right
		w.ue(0)      // top
		w.ue(cropBottom)
	} else {
		w.flag(false) // frame_cropping_flag
	}
	return w.bytes()
}

func TestParseSPS(t *testing.T) {
	cases := []struct {
		name         string
		rbsp         []byte
		wantW, wantH int
		wantProfile  uint8
	}{
		{"QCIF baseline 176x144", encodeSPS(66, 11, 10, 8, 0, false), 176, 144, 66},
		{"720p high 1280x720", encodeSPS(100, 31, 79, 44, 0, true), 1280, 720, 100},
		// 1080p: 1088 in map units, crop 8px from the bottom → cropBottom=4 (cropUnitY=2)
		{"1080p baseline 1920x1080", encodeSPS(66, 40, 119, 67, 4, false), 1920, 1080, 66},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sps, err := ParseSPS(c.rbsp)
			if err != nil {
				t.Fatalf("ParseSPS: %v", err)
			}
			if sps.Width != c.wantW || sps.Height != c.wantH {
				t.Errorf("size = %dx%d, want %dx%d", sps.Width, sps.Height, c.wantW, c.wantH)
			}
			if sps.ProfileIDC != c.wantProfile {
				t.Errorf("profile = %d, want %d", sps.ProfileIDC, c.wantProfile)
			}
			t.Logf("OK: %s", sps.Summary())
		})
	}
}

func TestParsePPS(t *testing.T) {
	w := &bitWriter{}
	w.ue(0)       // pic_parameter_set_id
	w.ue(0)       // seq_parameter_set_id
	w.flag(true)  // entropy_coding_mode_flag = CABAC
	w.flag(false) // bottom_field_pic_order_in_frame_present_flag
	w.ue(0)       // num_slice_groups_minus1 = 0 (no FMO)
	w.ue(0)       // num_ref_idx_l0_default_active_minus1
	w.ue(0)       // num_ref_idx_l1_default_active_minus1
	w.flag(false) // weighted_pred_flag
	w.u(2, 0)     // weighted_bipred_idc
	w.se(2)       // pic_init_qp_minus26 → base QP = 28
	w.se(0)       // pic_init_qs_minus26
	w.se(0)       // chroma_qp_index_offset
	w.flag(false) // deblocking_filter_control_present_flag
	w.flag(false) // constrained_intra_pred_flag
	w.flag(false) // redundant_pic_cnt_present_flag
	pps, err := ParsePPS(w.bytes())
	if err != nil {
		t.Fatalf("ParsePPS: %v", err)
	}
	if !pps.EntropyCodingMode || pps.EntropyName() != "CABAC" {
		t.Errorf("entropy = %s, want CABAC", pps.EntropyName())
	}
	if pps.PicInitQPMinus26 != 2 {
		t.Errorf("PicInitQPMinus26 = %d, want 2", pps.PicInitQPMinus26)
	}
}
