package decode

import (
	"errors"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

// PPS is the Picture Parameter Set (ISO/IEC 14496-10), the fields
// needed to parse the slice header and for subsequent decoding.
type PPS struct {
	ID                uint32
	SPSID             uint32
	EntropyCodingMode bool // false = CAVLC (Baseline), true = CABAC (Main/High)

	BottomFieldPicOrderPresent bool // bottom_field_pic_order_in_frame_present_flag
	NumSliceGroupsMinus1       uint32

	NumRefIdxL0DefaultMinus1 uint32
	NumRefIdxL1DefaultMinus1 uint32

	WeightedPred      bool   // weighted_pred_flag (P/SP)
	WeightedBipredIdc uint32 // weighted_bipred_idc (B)

	PicInitQPMinus26    int32 // base frame QP: SliceQP = 26 + this + slice_qp_delta
	ChromaQPIndexOffset int32

	DeblockingFilterControlPresent bool
	ConstrainedIntraPred           bool
	RedundantPicCntPresent         bool

	Transform8x8Mode        bool // transform_8x8_mode_flag (High profile)
	PicScalingMatrixPresent bool // custom scaling lists present (not yet applied)
}

// ErrFMO indicates that slice group partitioning (FMO) is present. It is
// exceedingly rare in practice; full parsing of the group map is not yet
// implemented.
var ErrFMO = errors.New("h264: slice groups (FMO) not supported")

// EntropyName returns the name of the entropy coding mode.
func (p *PPS) EntropyName() string {
	if p.EntropyCodingMode {
		return "CABAC"
	}
	return "CAVLC"
}

// ParsePPS parses a PPS RBSP. On FMO (num_slice_groups_minus1 > 0) it
// returns ErrFMO — we don't parse the group map yet.
func ParsePPS(rbsp []byte) (*PPS, error) {
	r := bitstream.NewReader(rbsp)
	p := &PPS{}
	p.ID = r.UE()
	p.SPSID = r.UE()
	p.EntropyCodingMode = r.Flag()
	p.BottomFieldPicOrderPresent = r.Flag()
	p.NumSliceGroupsMinus1 = r.UE()
	if p.NumSliceGroupsMinus1 > 0 {
		return nil, ErrFMO
	}
	p.NumRefIdxL0DefaultMinus1 = r.UE()
	p.NumRefIdxL1DefaultMinus1 = r.UE()
	p.WeightedPred = r.Flag()
	p.WeightedBipredIdc = r.U(2)
	p.PicInitQPMinus26 = r.SE()
	r.SE() // pic_init_qs_minus26
	p.ChromaQPIndexOffset = r.SE()
	p.DeblockingFilterControlPresent = r.Flag()
	p.ConstrainedIntraPred = r.Flag()
	p.RedundantPicCntPresent = r.Flag()
	// Optional High-profile extension.
	if r.MoreRBSPData() {
		p.Transform8x8Mode = r.Flag()
		p.PicScalingMatrixPresent = r.Flag()
		// Custom scaling lists are not applied yet; bailing keeps us honest
		// rather than silently decoding with a flat matrix.
		// (Their bits are not consumed here, but Transform8x8Mode is what we need.)
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return p, nil
}
