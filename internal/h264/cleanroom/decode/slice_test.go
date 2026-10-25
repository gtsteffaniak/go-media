package decode

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
)

// testSPS/testPPS are minimal parameter sets for testing slice header parsing.
func testSPS() *SPS {
	return &SPS{
		ProfileIDC:      66,
		Log2MaxFrameNum: 4, // minus4 = 0
		PicOrderCntType: 0,
		Log2MaxPocLsb:   4, // minus4 = 0
		FrameMbsOnly:    true,
		ChromaFormatIDC: 1,
	}
}

func testPPS() *PPS {
	return &PPS{
		PicInitQPMinus26: 0, // base QP = 26
	}
}

// encodeISliceHeader builds an IDR I-slice header in ParseSliceHeader order.
func encodeISliceHeader(sps *SPS, sliceQPDelta int32) []byte {
	w := &bitWriter{}
	w.ue(0)                     // first_mb_in_slice
	w.ue(uint32(SliceI))        // slice_type = 2 (I)
	w.ue(0)                     // pic_parameter_set_id
	w.u(sps.Log2MaxFrameNum, 0) // frame_num
	w.ue(0)                     // idr_pic_id (IDR)
	w.u(sps.Log2MaxPocLsb, 0)   // pic_order_cnt_lsb
	// nal_ref_idc != 0 → dec_ref_pic_marking (IDR): two flags
	w.flag(false) // no_output_of_prior_pics_flag
	w.flag(false) // long_term_reference_flag
	w.se(sliceQPDelta)
	return w.bytes()
}

func TestParseISliceHeader(t *testing.T) {
	sps, pps := testSPS(), testPPS()
	// slice_qp_delta = +4 → SliceQP = 26 + 0 + 4 = 30
	rbsp := encodeISliceHeader(sps, 4)
	h, err := ParseSliceHeader(rbsp, nal.TypeIDR, 3, sps, pps)
	if err != nil {
		t.Fatalf("ParseSliceHeader: %v", err)
	}
	if h.Type.Base() != SliceI || !h.Type.IsI() {
		t.Errorf("type = %s, want I", h.Type)
	}
	if h.SliceQP != 30 {
		t.Errorf("SliceQP = %d, want 30", h.SliceQP)
	}
	if h.FirstMB != 0 {
		t.Errorf("FirstMB = %d, want 0", h.FirstMB)
	}
}

// encodePSliceHeader is a non-IDR P-slice with num_ref_idx override and L0 list modification.
func encodePSliceHeader(sps *SPS) []byte {
	w := &bitWriter{}
	w.ue(0)                     // first_mb_in_slice
	w.ue(uint32(SliceP))        // slice_type = 0 (P)
	w.ue(0)                     // pps id
	w.u(sps.Log2MaxFrameNum, 5) // frame_num = 5
	w.u(sps.Log2MaxPocLsb, 10)  // pic_order_cnt_lsb = 10
	// P: num_ref_idx_active_override_flag
	w.flag(true)
	w.ue(0) // num_ref_idx_l0_active_minus1
	// ref_pic_list_modification (P, non-I)
	w.flag(true) // ref_pic_list_modification_flag_l0
	w.ue(0)      // modification_of_pic_nums_idc = 0
	w.ue(2)      // abs_diff_pic_num_minus1
	w.ue(3)      // idc = 3 → end
	// dec_ref_pic_marking (non-IDR): adaptive flag = 0
	w.flag(false)
	w.se(-2) // slice_qp_delta = -2 → SliceQP = 24
	return w.bytes()
}

func TestParsePSliceHeader(t *testing.T) {
	sps, pps := testSPS(), testPPS()
	rbsp := encodePSliceHeader(sps)
	h, err := ParseSliceHeader(rbsp, nal.TypeNonIDR, 2, sps, pps)
	if err != nil {
		t.Fatalf("ParseSliceHeader: %v", err)
	}
	if h.Type.Base() != SliceP {
		t.Errorf("type = %s, want P", h.Type)
	}
	if h.FrameNum != 5 || h.POCLsb != 10 {
		t.Errorf("frame_num=%d poc=%d, want 5 and 10", h.FrameNum, h.POCLsb)
	}
	if h.SliceQP != 24 {
		t.Errorf("SliceQP = %d, want 24", h.SliceQP)
	}
}

func TestPPSFMO(t *testing.T) {
	w := &bitWriter{}
	w.ue(0)       // pps id
	w.ue(0)       // sps id
	w.flag(false) // entropy
	w.flag(false) // bottom_field_pic_order...
	w.ue(1)       // num_slice_groups_minus1 = 1 → FMO
	if _, err := ParsePPS(w.bytes()); err != ErrFMO {
		t.Fatalf("want ErrFMO, got %v", err)
	}
}
