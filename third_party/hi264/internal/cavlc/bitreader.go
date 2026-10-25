// Package cavlc implements CAVLC (Context-Adaptive Variable-Length Coding)
// entropy decoding for H.264/AVC.
package cavlc

import (
	"fmt"
	"math/bits"
)

// BitReader reads Exp-Golomb and VLC symbols using a 64-bit bit cache (ffmpeg-style).
type BitReader struct {
	data    []byte
	bytePos int
	reg     uint64
	valid   int // number of valid bits in reg (MSB-aligned)
}

// NewBitReader creates a new BitReader.
func NewBitReader(data []byte) *BitReader {
	r := &BitReader{data: data}
	r.load()
	return r
}

func (r *BitReader) load() {
	for r.valid <= 56 && r.bytePos < len(r.data) {
		r.reg |= uint64(r.data[r.bytePos]) << (56 - r.valid)
		r.bytePos++
		r.valid += 8
	}
}

func (r *BitReader) ensure(n int) error {
	for r.valid < n {
		if r.bytePos >= len(r.data) {
			return fmt.Errorf("bitreader: end of data at byte %d", r.bytePos)
		}
		r.load()
	}
	return nil
}

// BitsRead returns the total number of bits consumed from the stream.
func (r *BitReader) BitsRead() int {
	unread := r.valid
	consumedBits := r.bytePos*8 - unread
	if consumedBits < 0 {
		consumedBits = 0
	}
	return consumedBits
}

func (r *BitReader) take(n int) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if err := r.ensure(n); err != nil {
		return 0, err
	}
	v := uint32(r.reg >> (64 - n))
	r.reg <<= n
	r.valid -= n
	r.load()
	return v, nil
}

// ReadBit reads a single bit.
func (r *BitReader) ReadBit() (uint8, error) {
	v, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return uint8(v), nil
}

// ReadBits reads n bits and returns them as a uint32.
func (r *BitReader) ReadBits(n int) (uint32, error) {
	if n > 32 || n < 0 {
		return 0, fmt.Errorf("bitreader: invalid n=%d", n)
	}
	return r.take(n)
}

// ReadFlag reads a single bit as a boolean.
func (r *BitReader) ReadFlag() (bool, error) {
	bit, err := r.ReadBit()
	return bit == 1, err
}

// PeekBits reads n bits without advancing the position.
// If fewer than n bits remain, the available bits are left-shifted (padded with zeros on the right).
func (r *BitReader) PeekBits(n int) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if n > 32 || n < 0 {
		return 0, fmt.Errorf("bitreader: invalid n=%d", n)
	}
	savedReg := r.reg
	savedValid := r.valid
	savedPos := r.bytePos
	var val uint32
	bitsRead := 0
	for bitsRead < n {
		r.load()
		if r.valid == 0 {
			val <<= uint(n - bitsRead)
			break
		}
		take := n - bitsRead
		if take > r.valid {
			take = r.valid
		}
		chunk := uint32(r.reg >> (64 - take))
		val = (val << uint(take)) | chunk
		r.reg <<= take
		r.valid -= take
		bitsRead += take
	}
	r.reg = savedReg
	r.valid = savedValid
	r.bytePos = savedPos
	return val, nil
}

// SkipBits advances the position by n bits.
func (r *BitReader) SkipBits(n int) {
	for n > 0 {
		chunk := n
		if chunk > r.valid {
			chunk = r.valid
		}
		if chunk == 0 {
			if r.bytePos >= len(r.data) {
				return
			}
			r.load()
			continue
		}
		r.reg <<= chunk
		r.valid -= chunk
		n -= chunk
		r.load()
	}
}

// ReadUE reads an unsigned Exp-Golomb coded value (ue(v)).
func (r *BitReader) ReadUE() (uint32, error) {
	r.load()
	leadingZeros := 0
	for {
		if r.valid == 0 {
			if r.bytePos >= len(r.data) {
				return 0, fmt.Errorf("ue(v): end of data")
			}
			r.load()
		}
		lz := bits.LeadingZeros64(r.reg)
		if lz >= r.valid {
			leadingZeros += r.valid
			r.reg = 0
			r.valid = 0
			r.load()
			if r.valid == 0 && r.bytePos >= len(r.data) {
				return 0, fmt.Errorf("ue(v): end of data")
			}
			continue
		}
		leadingZeros += lz
		r.reg <<= lz + 1
		r.valid -= lz + 1
		r.load()
		break
	}
	if leadingZeros == 0 {
		return 0, nil
	}
	suffix, err := r.take(leadingZeros)
	if err != nil {
		return 0, fmt.Errorf("ue(v) suffix: %w", err)
	}
	return (1 << uint(leadingZeros)) - 1 + suffix, nil
}

// ReadSE reads a signed Exp-Golomb coded value (se(v)).
func (r *BitReader) ReadSE() (int32, error) {
	ue, err := r.ReadUE()
	if err != nil {
		return 0, err
	}
	if ue%2 == 0 {
		return -int32(ue / 2), nil
	}
	return int32((ue + 1) / 2), nil
}

// AlignToByte advances to the next byte boundary.
func (r *BitReader) AlignToByte() {
	skip := r.valid % 8
	if skip > 0 {
		r.SkipBits(skip)
	}
}

// SliceHeaderParams holds parameters needed to skip the IDR slice header.
type SliceHeaderParams struct {
	FrameMbsOnly                          bool
	Log2MaxFrameNumMinus4                 uint
	PicOrderCntType                       uint
	Log2MaxPicOrderCntLsbMinus4           uint
	BottomFieldPicOrderInFramePresentFlag bool
	DeblockingFilterControlPresent        bool
	RedundantPicCntPresentFlag            bool
	NumSliceGroupsMinus1                  uint
}

// SkipSliceHeaderIDR advances the reader past an IDR I-slice header to the
// start of slice data. The NALU data (starting with the NAL header byte) must
// be at position 0 in the reader.
func (r *BitReader) SkipSliceHeaderIDR(p SliceHeaderParams) error {
	_, err := r.ReadBits(8)
	if err != nil {
		return fmt.Errorf("nal header: %w", err)
	}
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("first_mb_in_slice: %w", err)
	}
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("slice_type: %w", err)
	}
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("pps_id: %w", err)
	}
	_, err = r.ReadBits(int(p.Log2MaxFrameNumMinus4 + 4))
	if err != nil {
		return fmt.Errorf("frame_num: %w", err)
	}
	fieldPicFlag := false
	if !p.FrameMbsOnly {
		fieldPicFlag, err = r.ReadFlag()
		if err != nil {
			return fmt.Errorf("field_pic_flag: %w", err)
		}
		if fieldPicFlag {
			_, err = r.ReadBit()
			if err != nil {
				return fmt.Errorf("bottom_field_flag: %w", err)
			}
		}
	}
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("idr_pic_id: %w", err)
	}
	if p.PicOrderCntType == 0 {
		_, err = r.ReadBits(int(p.Log2MaxPicOrderCntLsbMinus4 + 4))
		if err != nil {
			return fmt.Errorf("pic_order_cnt_lsb: %w", err)
		}
		if p.BottomFieldPicOrderInFramePresentFlag && !fieldPicFlag {
			_, err = r.ReadSE()
			if err != nil {
				return fmt.Errorf("delta_pic_order_cnt_bottom: %w", err)
			}
		}
	}
	if p.RedundantPicCntPresentFlag {
		_, err = r.ReadUE()
		if err != nil {
			return fmt.Errorf("redundant_pic_cnt: %w", err)
		}
	}
	_, err = r.ReadBit()
	if err != nil {
		return fmt.Errorf("no_output_of_prior_pics_flag: %w", err)
	}
	_, err = r.ReadBit()
	if err != nil {
		return fmt.Errorf("long_term_reference_flag: %w", err)
	}
	_, err = r.ReadSE()
	if err != nil {
		return fmt.Errorf("slice_qp_delta: %w", err)
	}
	if p.DeblockingFilterControlPresent {
		disableDeblockingFilterIdc, err := r.ReadUE()
		if err != nil {
			return fmt.Errorf("disable_deblocking_filter_idc: %w", err)
		}
		if disableDeblockingFilterIdc != 1 {
			_, err = r.ReadSE()
			if err != nil {
				return fmt.Errorf("slice_alpha: %w", err)
			}
			_, err = r.ReadSE()
			if err != nil {
				return fmt.Errorf("slice_beta: %w", err)
			}
		}
	}
	return nil
}

// SkipSliceHeaderP advances the reader past a non-IDR P-slice header.
func (r *BitReader) SkipSliceHeaderP(p SliceHeaderParams, nalRefIdc uint8) error {
	_, err := r.ReadBits(8)
	if err != nil {
		return fmt.Errorf("nal header: %w", err)
	}
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("first_mb_in_slice: %w", err)
	}
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("slice_type: %w", err)
	}
	_, err = r.ReadUE()
	if err != nil {
		return fmt.Errorf("pps_id: %w", err)
	}
	_, err = r.ReadBits(int(p.Log2MaxFrameNumMinus4 + 4))
	if err != nil {
		return fmt.Errorf("frame_num: %w", err)
	}
	fieldPicFlag := false
	if !p.FrameMbsOnly {
		fieldPicFlag, err = r.ReadFlag()
		if err != nil {
			return fmt.Errorf("field_pic_flag: %w", err)
		}
		if fieldPicFlag {
			_, err = r.ReadBit()
			if err != nil {
				return fmt.Errorf("bottom_field_flag: %w", err)
			}
		}
	}
	if p.PicOrderCntType == 0 {
		_, err = r.ReadBits(int(p.Log2MaxPicOrderCntLsbMinus4 + 4))
		if err != nil {
			return fmt.Errorf("pic_order_cnt_lsb: %w", err)
		}
		if p.BottomFieldPicOrderInFramePresentFlag && !fieldPicFlag {
			_, err = r.ReadSE()
			if err != nil {
				return fmt.Errorf("delta_pic_order_cnt_bottom: %w", err)
			}
		}
	}
	if p.RedundantPicCntPresentFlag {
		_, err = r.ReadUE()
		if err != nil {
			return fmt.Errorf("redundant_pic_cnt: %w", err)
		}
	}
	overrideFlag, err := r.ReadFlag()
	if err != nil {
		return fmt.Errorf("num_ref_idx_active_override_flag: %w", err)
	}
	if overrideFlag {
		_, err = r.ReadUE()
		if err != nil {
			return fmt.Errorf("num_ref_idx_l0_active_minus1: %w", err)
		}
	}
	rplmFlag, err := r.ReadFlag()
	if err != nil {
		return fmt.Errorf("ref_pic_list_modification_flag_l0: %w", err)
	}
	if rplmFlag {
		for {
			op, err := r.ReadUE()
			if err != nil {
				return fmt.Errorf("modification_of_pic_nums_idc: %w", err)
			}
			if op == 3 {
				break
			}
			_, err = r.ReadUE()
			if err != nil {
				return fmt.Errorf("rplm operand: %w", err)
			}
		}
	}
	if nalRefIdc != 0 {
		adaptiveFlag, err := r.ReadFlag()
		if err != nil {
			return fmt.Errorf("adaptive_ref_pic_marking_mode_flag: %w", err)
		}
		if adaptiveFlag {
			for {
				op, err := r.ReadUE()
				if err != nil {
					return fmt.Errorf("mmco op: %w", err)
				}
				if op == 0 {
					break
				}
				_, err = r.ReadUE()
				if err != nil {
					return fmt.Errorf("mmco operand: %w", err)
				}
				if op == 3 {
					_, err = r.ReadUE()
					if err != nil {
						return fmt.Errorf("mmco op3 operand: %w", err)
					}
				}
			}
		}
	}
	_, err = r.ReadSE()
	if err != nil {
		return fmt.Errorf("slice_qp_delta: %w", err)
	}
	if p.DeblockingFilterControlPresent {
		disableDeblockingFilterIdc, err := r.ReadUE()
		if err != nil {
			return fmt.Errorf("disable_deblocking_filter_idc: %w", err)
		}
		if disableDeblockingFilterIdc != 1 {
			_, err = r.ReadSE()
			if err != nil {
				return fmt.Errorf("slice_alpha: %w", err)
			}
			_, err = r.ReadSE()
			if err != nil {
				return fmt.Errorf("slice_beta: %w", err)
			}
		}
	}
	return nil
}
