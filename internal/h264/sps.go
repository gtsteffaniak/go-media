package h264

import (
	"errors"
	"fmt"
	"io"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// gatedProfiles maps profile_idc to the reason a profile that decodes correctly is not
// enabled: it has not met the ffmpeg thumbnail timing gate. It is empty today (H.264
// Baseline/CAVLC, profile_idc 66, met the gate and was enabled). The comparison harness
// decodes gated profiles through AllowGatedProfiles and fails when one starts to pass, so
// an entry here is removed as soon as the measurement says it can be.
var gatedProfiles = map[int]string{}

// validateSPSNAL rejects profiles go-media does not decode for thumbnails.
func validateSPSNAL(nal []byte) error {
	sps, err := parseSPSNAL(nal)
	if err != nil {
		return errs.UnsupProfile("h264", fmt.Sprintf("sps: %v", err))
	}
	if reason, gated := gatedProfiles[int(sps.profile)]; gated && !allowGatedProfiles.Load() {
		return errs.UnsupProfile("h264", reason)
	}
	if sps.chromaFormatIDC != 1 {
		return errs.UnsupProfile("h264", fmt.Sprintf("chroma format %d (need 4:2:0)", sps.chromaFormatIDC))
	}
	if sps.bitDepthLumaMinus8 > 2 || sps.bitDepthChromaMinus8 > 2 {
		// 9/10-bit 4:2:0 is reconstructed on a scalar uint16 path; deeper
		// samples (High 4:4:4 Predictive, 12/14-bit) are not.
		return errs.UnsupProfile("h264", fmt.Sprintf("%d-bit luma / %d-bit chroma (profile_idc %d); decoder supports 8, 9 and 10 bits",
			sps.bitDepthLumaMinus8+8, sps.bitDepthChromaMinus8+8, sps.profile))
	}
	// High profile 8x8 transform and scaling lists are supported.
	if sps.mbAdaptiveFrameField {
		return errs.UnsupProfile("h264", "MBAFF")
	}
	if !sps.frameMbsOnly {
		return errs.UnsupProfile("h264", "interlaced frame encoding")
	}
	return nil
}

// spsFields holds the sequence parameter set fields validateSPSNAL checks.
type spsFields struct {
	profile              uint32
	chromaFormatIDC      byte
	bitDepthLumaMinus8   uint
	bitDepthChromaMinus8 uint
	frameMbsOnly         bool
	mbAdaptiveFrameField bool
	// pocCycleLen is num_ref_frames_in_pic_order_cnt_cycle as read (0 unless
	// pic_order_cnt_type is 1). It is kept for tests.
	pocCycleLen uint
}

// errNotSPS is returned for a NAL unit whose nal_unit_type is not 7.
var errNotSPS = errors.New("not an SPS NAL unit")

// parseSPSNAL parses an SPS NAL unit, header byte included, per ITU-T H.264
// 7.3.2.1.1. It reads every syntax element up to and including the VUI aspect
// ratio, so a unit cut short anywhere in that range fails with io.EOF, and the
// error cases match the mp4ff parser go-media used before: a non-SPS NAL type, a
// chroma_format_idc above 3 when frame cropping is present, and a reserved
// aspect_ratio_idc.
func parseSPSNAL(nal []byte) (spsFields, error) {
	var s spsFields
	r := ebspReader{data: nal}
	if r.read(8)&0x1F != 7 {
		return s, errNotSPS
	}
	s.profile = uint32(r.read(8))
	r.read(8)  // constraint flags
	r.read(8)  // level_idc
	r.readUE() // seq_parameter_set_id
	s.chromaFormatIDC = 1
	if s.profile == 138 {
		s.chromaFormatIDC = 0
	}
	switch s.profile {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		s.chromaFormatIDC = byte(r.readUE())
		if s.chromaFormatIDC == 3 {
			r.readFlag() // separate_colour_plane_flag
		}
		s.bitDepthLumaMinus8 = r.readUE()
		s.bitDepthChromaMinus8 = r.readUE()
		r.readFlag()      // qpprime_y_zero_transform_bypass_flag
		if r.readFlag() { // seq_scaling_matrix_present_flag
			lists := 8
			if s.chromaFormatIDC == 3 {
				lists = 12
			}
			for i := 0; i < lists; i++ {
				if !r.readFlag() {
					continue
				}
				size := 16
				if i >= 6 {
					size = 64
				}
				r.skipScalingList(size)
			}
		}
	}

	r.readUE()          // log2_max_frame_num_minus4
	switch r.readUE() { // pic_order_cnt_type
	case 0:
		r.readUE() // log2_max_pic_order_cnt_lsb_minus4
	case 1:
		r.readFlag() // delta_pic_order_always_zero_flag
		r.readUE()   // offset_for_non_ref_pic
		r.readUE()   // offset_for_top_to_bottom_field
		cycle := r.readUE()
		s.pocCycleLen = cycle
		// Every entry takes at least one bit, so a count above the bits left
		// cannot be read in full.
		if cycle > r.bitsLeft() {
			r.fail(io.EOF)
		}
		for i := uint(0); i < cycle && r.err == nil; i++ {
			r.readUE() // offset_for_ref_frame[i]
		}
	}
	r.readUE()   // max_num_ref_frames
	r.readFlag() // gaps_in_frame_num_value_allowed_flag
	r.readUE()   // pic_width_in_mbs_minus1
	r.readUE()   // pic_height_in_map_units_minus1
	s.frameMbsOnly = r.readFlag()
	if !s.frameMbsOnly {
		s.mbAdaptiveFrameField = r.readFlag()
	}
	r.readFlag()      // direct_8x8_inference_flag
	if r.readFlag() { // frame_cropping_flag
		if s.chromaFormatIDC > 3 {
			return s, fmt.Errorf("invalid chroma_format_idc value: %d", s.chromaFormatIDC)
		}
		for range 4 {
			r.readUE() // frame_crop_{left,right,top,bottom}_offset
		}
	}
	if r.readFlag() { // vui_parameters_present_flag
		if r.readFlag() { // aspect_ratio_info_present_flag
			switch idc := r.read(8); {
			case idc == 255: // Extended_SAR
				r.read(16) // sar_width
				r.read(16) // sar_height
			case idc > 16 && r.err == nil:
				r.fail(fmt.Errorf("SAR bad index %d", idc))
			}
		}
	}
	return s, r.err
}

// ebspReader reads bits from a NAL unit and drops each emulation prevention byte
// (0x03 after exactly two 0x00 bytes). The first error sticks: later reads
// return 0 and leave it unchanged.
type ebspReader struct {
	data  []byte
	off   int   // next byte of data
	v     uint  // buffered bits, right-aligned
	n     int   // number of buffered bits in v
	zeros int   // count of consecutive 0x00 bytes just read
	err   error // first error
}

// fail records err unless an earlier error is already recorded.
func (r *ebspReader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

// nextByte returns the next raw byte, or false at the end of the data.
func (r *ebspReader) nextByte() (byte, bool) {
	if r.off >= len(r.data) {
		return 0, false
	}
	b := r.data[r.off]
	r.off++
	return b, true
}

// read returns the next n bits as an unsigned value. For n above the width of
// uint only the low bits are kept, in the same way mp4ff's EBSP reader did.
func (r *ebspReader) read(n int) uint {
	if r.err != nil {
		return 0
	}
	for r.n < n {
		r.v <<= 8
		b, ok := r.nextByte()
		if !ok {
			r.err = io.EOF
			return 0
		}
		if r.zeros == 2 && b == 0x03 {
			if b, ok = r.nextByte(); !ok {
				r.err = io.EOF
				return 0
			}
			r.zeros = 0
		}
		if b != 0 {
			r.zeros = 0
		} else {
			r.zeros++
		}
		r.v |= uint(b)
		r.n += 8
	}
	v := r.v >> uint(r.n-n)
	r.n -= n
	r.v &= 1<<uint(r.n) - 1
	return v
}

// readFlag reads a one-bit flag.
func (r *ebspReader) readFlag() bool {
	return r.read(1) == 1
}

// readUE reads an unsigned Exp-Golomb value, ue(v). The arithmetic wraps for
// prefixes longer than the width of uint, as mp4ff's reader did.
func (r *ebspReader) readUE() uint {
	if r.err != nil {
		return 0
	}
	zeros := 0
	for {
		b := r.read(1)
		if r.err != nil {
			return 0
		}
		if b == 1 {
			break
		}
		zeros++
	}
	base := uint(1)<<zeros - 1
	rest := r.read(zeros)
	if r.err != nil {
		return 0
	}
	return base + rest
}

// readSE reads a signed Exp-Golomb value, se(v).
func (r *ebspReader) readSE() int {
	u := r.readUE()
	if r.err != nil {
		return 0
	}
	if u%2 == 1 {
		return int((u + 1) / 2)
	}
	return -int(u / 2)
}

// skipScalingList reads one scaling_list() of the given size (7.3.2.1.1.1). The
// values are not kept; only the bits consumed matter.
func (r *ebspReader) skipScalingList(size int) {
	last, next := 8, 8
	for j := 0; j < size; j++ {
		if next != 0 {
			next = (last + r.readSE() + 256) % 256
		}
		if next != 0 {
			last = next
		}
	}
}

// bitsLeft returns an upper bound on the bits still readable: the buffered bits
// plus 8 per unread byte, emulation prevention bytes included.
func (r *ebspReader) bitsLeft() uint {
	return uint(r.n) + 8*uint(len(r.data)-r.off)
}
