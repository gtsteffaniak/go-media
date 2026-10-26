package decode

import (
	"errors"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
)

// SliceType is the slice type. Values 5..9 mean "all slices of the frame have
// the same type"; the base type is obtained via %5.
type SliceType uint32

const (
	SliceP  SliceType = 0
	SliceB  SliceType = 1
	SliceI  SliceType = 2
	SliceSP SliceType = 3
	SliceSI SliceType = 4
)

// Base normalizes the type to 0..4.
func (t SliceType) Base() SliceType { return t % 5 }

// IsI reports whether the slice is intra (I or SI) — without inter prediction.
func (t SliceType) IsI() bool { b := t.Base(); return b == SliceI || b == SliceSI }

// String gives a short name for the slice type.
func (t SliceType) String() string {
	switch t.Base() {
	case SliceP:
		return "P"
	case SliceB:
		return "B"
	case SliceI:
		return "I"
	case SliceSP:
		return "SP"
	default:
		return "SI"
	}
}

// SliceHeader is the parsed slice header, the fields needed to
// decode macroblocks.
type SliceHeader struct {
	FirstMB      uint32
	Type         SliceType
	PPSID        uint32
	FrameNum     uint32
	FieldPic     bool
	BottomField  bool
	IDRPicID     uint32
	POCLsb       uint32
	SliceQP      int32 // 26 + pic_init_qp_minus26 + slice_qp_delta
	CabacInitIdc uint32

	DisableDeblock uint32 // disable_deblocking_filter_idc (0/1/2)
	AlphaC0Offset  int32
	BetaOffset     int32
}

// ErrParamSetMismatch means the slice references a PPS/SPS that is not in the context.
var ErrParamSetMismatch = errors.New("h264: slice references an unknown PPS/SPS")

// ParseSliceHeader parses the slice header (without the data that follows).
// nalType determines IdrPicFlag, nalRefIDC whether dec_ref_pic_marking is needed.
func ParseSliceHeader(rbsp []byte, nalType nal.Type, nalRefIDC uint8, sps *SPS, pps *PPS) (*SliceHeader, error) {
	r := bitstream.NewReader(rbsp)
	h, err := readSliceHeader(r, nalType, nalRefIDC, sps, pps)
	if err != nil {
		return nil, err
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return h, nil
}

// readSliceHeader parses the header on top of an existing reader, leaving the
// position at the start of slice_data — the decoder needs this to continue reading.
func readSliceHeader(r *bitstream.Reader, nalType nal.Type, nalRefIDC uint8, sps *SPS, pps *PPS) (*SliceHeader, error) {
	if sps == nil || pps == nil {
		return nil, ErrParamSetMismatch
	}
	idr := nalType == nal.TypeIDR
	h := &SliceHeader{}

	h.FirstMB = r.UE()
	h.Type = SliceType(r.UE())
	h.PPSID = r.UE()

	if sps.SeparateColourPlane {
		r.U(2) // colour_plane_id
	}
	h.FrameNum = r.U(sps.Log2MaxFrameNum)

	if !sps.FrameMbsOnly {
		h.FieldPic = r.Flag()
		if h.FieldPic {
			h.BottomField = r.Flag()
		}
	}
	if idr {
		h.IDRPicID = r.UE()
	}

	if sps.PicOrderCntType == 0 {
		h.POCLsb = r.U(sps.Log2MaxPocLsb)
		if pps.BottomFieldPicOrderPresent && !h.FieldPic {
			r.SE() // delta_pic_order_cnt_bottom
		}
	} else if sps.PicOrderCntType == 1 && !sps.DeltaPicOrderAlwaysZero {
		r.SE() // delta_pic_order_cnt[0]
		if pps.BottomFieldPicOrderPresent && !h.FieldPic {
			r.SE() // delta_pic_order_cnt[1]
		}
	}

	if pps.RedundantPicCntPresent {
		r.UE() // redundant_pic_cnt
	}

	base := h.Type.Base()
	if base == SliceB {
		r.Flag() // direct_spatial_mv_pred_flag
	}

	numRefL0 := pps.NumRefIdxL0DefaultMinus1
	numRefL1 := pps.NumRefIdxL1DefaultMinus1
	if base == SliceP || base == SliceSP || base == SliceB {
		if r.Flag() { // num_ref_idx_active_override_flag
			numRefL0 = r.UE()
			if base == SliceB {
				numRefL1 = r.UE()
			}
		}
	}

	parseRefPicListModification(r, base)

	if (pps.WeightedPred && (base == SliceP || base == SliceSP)) ||
		(pps.WeightedBipredIdc == 1 && base == SliceB) {
		parsePredWeightTable(r, sps, base, numRefL0, numRefL1)
	}

	if nalRefIDC != 0 {
		parseDecRefPicMarking(r, idr)
	}

	if pps.EntropyCodingMode && !base.IsI() {
		h.CabacInitIdc = r.UE()
	}

	sliceQPDelta := r.SE()
	h.SliceQP = 26 + pps.PicInitQPMinus26 + sliceQPDelta

	if base == SliceSP || base == SliceSI {
		if base == SliceSP {
			r.Flag() // sp_for_switch_flag
		}
		r.SE() // slice_qs_delta
	}

	if pps.DeblockingFilterControlPresent {
		h.DisableDeblock = r.UE()
		if h.DisableDeblock != 1 {
			h.AlphaC0Offset = r.SE() * 2
			h.BetaOffset = r.SE() * 2
		}
	}

	if err := r.Err(); err != nil {
		return nil, err
	}
	return h, nil
}

// parseRefPicListModification.
func parseRefPicListModification(r *bitstream.Reader, base SliceType) {
	if !base.IsI() {
		if r.Flag() { // ref_pic_list_modification_flag_l0
			for {
				idc := r.UE()
				if idc == 3 {
					break
				}
				if idc == 0 || idc == 1 {
					r.UE() // abs_diff_pic_num_minus1
				} else if idc == 2 {
					r.UE() // long_term_pic_num
				}
				if r.Err() != nil {
					return
				}
			}
		}
	}
	if base == SliceB {
		if r.Flag() { // ref_pic_list_modification_flag_l1
			for {
				idc := r.UE()
				if idc == 3 {
					break
				}
				if idc == 0 || idc == 1 {
					r.UE()
				} else if idc == 2 {
					r.UE()
				}
				if r.Err() != nil {
					return
				}
			}
		}
	}
}

// parsePredWeightTable.
func parsePredWeightTable(r *bitstream.Reader, sps *SPS, base SliceType, numRefL0, numRefL1 uint32) {
	r.UE() // luma_log2_weight_denom
	hasChroma := sps.ChromaArrayType() != 0
	if hasChroma {
		r.UE() // chroma_log2_weight_denom
	}
	readList := func(n uint32) {
		for i := uint32(0); i <= n; i++ {
			if r.Flag() { // luma_weight_lX_flag
				r.SE() // luma_weight
				r.SE() // luma_offset
			}
			if hasChroma {
				if r.Flag() { // chroma_weight_lX_flag
					for j := 0; j < 2; j++ {
						r.SE() // chroma_weight
						r.SE() // chroma_offset
					}
				}
			}
			if r.Err() != nil {
				return
			}
		}
	}
	readList(numRefL0)
	if base == SliceB {
		readList(numRefL1)
	}
}

// parseDecRefPicMarking.
func parseDecRefPicMarking(r *bitstream.Reader, idr bool) {
	if idr {
		r.Flag() // no_output_of_prior_pics_flag
		r.Flag() // long_term_reference_flag
		return
	}
	if r.Flag() { // adaptive_ref_pic_marking_mode_flag
		for {
			op := r.UE()
			if op == 0 {
				break
			}
			if op == 1 || op == 3 {
				r.UE() // difference_of_pic_nums_minus1
			}
			if op == 2 {
				r.UE() // long_term_pic_num
			}
			if op == 3 || op == 6 {
				r.UE() // long_term_frame_idx
			}
			if op == 4 {
				r.UE() // max_long_term_frame_idx_plus1
			}
			if r.Err() != nil {
				return
			}
		}
	}
}
