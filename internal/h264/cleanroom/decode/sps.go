// Package h264 parses H.264 syntax on top of bitstream/nal. So far it covers
// the parameter sets (SPS/PPS), enough for frame dimensions and to determine
// the profile/entropy coding. Decoding the macroblocks themselves is for later
// phases.
package decode

import (
	"fmt"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

// SPS is the subset of the Sequence Parameter Set (ISO/IEC 14496-10)
// needed for frame dimensions, chroma format and profile.
type SPS struct {
	ProfileIDC uint8
	LevelIDC   uint8
	ID         uint32

	ChromaFormatIDC     uint32 // 0=mono, 1=4:2:0, 2=4:2:2, 3=4:4:4
	SeparateColourPlane bool
	BitDepthLuma        uint32
	BitDepthChroma      uint32

	// Fields needed to parse the slice header.
	Log2MaxFrameNum         int // log2_max_frame_num_minus4 + 4
	PicOrderCntType         uint32
	Log2MaxPocLsb           int  // log2_max_pic_order_cnt_lsb_minus4 + 4 (type 0)
	DeltaPicOrderAlwaysZero bool // type 1

	FrameMbsOnly   bool
	WidthMbs       uint32 // pic_width_in_mbs
	HeightMapUnits uint32 // pic_height_in_map_units

	CropLeft, CropRight, CropTop, CropBottom uint32

	// Computed (after cropping), in pixels.
	Width   int
	Height  int
	OffsetX int // luma pixel origin of the visible (cropped) region
	OffsetY int

	// Colour signalling from the VUI video_signal_type (E.1.1). Only these
	// fields are read; the rest of the VUI is ignored. When the VUI is absent
	// or truncated they stay zero, which callers treat as "untagged" just like
	// ffmpeg's AVCOL_SPC_UNSPECIFIED / AVCOL_RANGE_UNSPECIFIED.
	// VUI video_signal_type, used for thumbnail colour like ffmpeg does.
	// All zero when the SPS has no VUI or the VUI is truncated (untagged).
	VideoFullRange     bool  // video_full_range_flag
	ColourDescription  bool  // colour_description_present_flag
	MatrixCoefficients uint8 // matrix_coefficients (2 = unspecified)
}

// isHighProfile lists the profiles whose SPS contains the
// chroma_format_idc / bit_depth / scaling matrix block.
func isHighProfile(p uint8) bool {
	switch p {
	case 100, 110, 122, 244, 44, 83, 86, 118, 128, 138, 139, 134, 135:
		return true
	}
	return false
}

// ParseSPS parses an SPS RBSP (without the NAL header byte).
func ParseSPS(rbsp []byte) (*SPS, error) {
	r := bitstream.NewReader(rbsp)
	s := &SPS{
		ChromaFormatIDC: 1, // default 4:2:0
		BitDepthLuma:    8,
		BitDepthChroma:  8,
	}

	s.ProfileIDC = uint8(r.U(8))
	r.U(8) // constraint_set0..5_flag + reserved_zero_2bits
	s.LevelIDC = uint8(r.U(8))
	s.ID = r.UE()
	if s.ID > maxSeqParameterSetID {
		return nil, fmt.Errorf("h264 sps: seq_parameter_set_id %d exceeds maximum %d", s.ID, maxSeqParameterSetID)
	}

	if isHighProfile(s.ProfileIDC) {
		s.ChromaFormatIDC = r.UE()
		if s.ChromaFormatIDC > maxChromaFormatIDC {
			return nil, fmt.Errorf("h264 sps: chroma_format_idc %d exceeds maximum %d", s.ChromaFormatIDC, maxChromaFormatIDC)
		}
		if s.ChromaFormatIDC == 3 {
			s.SeparateColourPlane = r.Flag()
		}
		bitDepthLumaMinus8 := r.UE()
		bitDepthChromaMinus8 := r.UE()
		if bitDepthLumaMinus8 > maxBitDepth-8 || bitDepthChromaMinus8 > maxBitDepth-8 {
			return nil, fmt.Errorf("h264 sps: bit depth luma_minus8=%d chroma_minus8=%d exceeds maximum %d", bitDepthLumaMinus8, bitDepthChromaMinus8, maxBitDepth-8)
		}
		s.BitDepthLuma = bitDepthLumaMinus8 + 8
		s.BitDepthChroma = bitDepthChromaMinus8 + 8
		r.Flag()      // qpprime_y_zero_transform_bypass_flag
		if r.Flag() { // seq_scaling_matrix_present_flag
			n := 8
			if s.ChromaFormatIDC == 3 {
				n = 12
			}
			for i := 0; i < n; i++ {
				if r.Flag() { // seq_scaling_list_present_flag[i]
					size := 16
					if i >= 6 {
						size = 64
					}
					skipScalingList(r, size)
				}
			}
		}
	}

	log2FN := r.UE()
	if log2FN > maxLog2MaxFrameNumMinus4 {
		return nil, fmt.Errorf("h264 sps: log2_max_frame_num_minus4 %d exceeds maximum %d", log2FN, maxLog2MaxFrameNumMinus4)
	}
	s.Log2MaxFrameNum = int(log2FN) + 4
	s.PicOrderCntType = r.UE()
	switch s.PicOrderCntType {
	case 0:
		log2POC := r.UE()
		if log2POC > maxLog2MaxPicOrderCntLsbMinus4 {
			return nil, fmt.Errorf("h264 sps: log2_max_pic_order_cnt_lsb_minus4 %d exceeds maximum %d", log2POC, maxLog2MaxPicOrderCntLsbMinus4)
		}
		s.Log2MaxPocLsb = int(log2POC) + 4
	case 1:
		s.DeltaPicOrderAlwaysZero = r.Flag()
		r.SE() // offset_for_non_ref_pic
		if err := r.Err(); err != nil {
			return nil, fmt.Errorf("h264 sps: %w", err)
		}
		r.SE() // offset_for_top_to_bottom_field
		if err := r.Err(); err != nil {
			return nil, fmt.Errorf("h264 sps: %w", err)
		}
		num := r.UE()
		if num > maxNumRefFramesInPicOrderCntCycle {
			return nil, fmt.Errorf("h264 sps: num_ref_frames_in_pic_order_cnt_cycle %d exceeds maximum %d", num, maxNumRefFramesInPicOrderCntCycle)
		}
		for i := uint32(0); i < num; i++ {
			r.SE() // offset_for_ref_frame[i]
			if err := r.Err(); err != nil {
				return nil, fmt.Errorf("h264 sps: %w", err)
			}
		}
	}

	r.UE()   // max_num_ref_frames
	r.Flag() // gaps_in_frame_num_value_allowed_flag

	s.WidthMbs = r.UE() + 1
	s.HeightMapUnits = r.UE() + 1
	s.FrameMbsOnly = r.Flag()
	if !s.FrameMbsOnly {
		r.Flag() // mb_adaptive_frame_field_flag
	}
	r.Flag() // direct_8x8_inference_flag

	if r.Flag() { // frame_cropping_flag
		s.CropLeft = r.UE()
		s.CropRight = r.UE()
		s.CropTop = r.UE()
		s.CropBottom = r.UE()
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("h264 sps: %w", err)
	}
	if r.Flag() { // vui_parameters_present_flag
		parseVUIColour(r, s)
	}
	if err := checkMacroblockBudget(s); err != nil {
		return nil, err
	}
	if err := s.computeDimensions(); err != nil {
		return nil, err
	}
	return s, nil
}

// parseVUIColour reads the VUI up to video_signal_type and stores the range and
// matrix. ffmpeg's h264_ps.c reads the same fields to set color_range and
// colorspace on the decoded frame, which swscale then uses. A VUI that is
// truncated before these fields leaves them untagged instead of failing the
// SPS: the colour fields are advisory and the picture is still decodable.
func parseVUIColour(r *bitstream.Reader, s *SPS) {
	if r.Flag() { // aspect_ratio_info_present_flag
		if r.U(8) == 255 { // aspect_ratio_idc == Extended_SAR
			r.U(16) // sar_width
			r.U(16) // sar_height
		}
	}
	if r.Flag() { // overscan_info_present_flag
		r.Flag() // overscan_appropriate_flag
	}
	if !r.Flag() { // video_signal_type_present_flag
		return
	}
	r.U(3) // video_format
	full := r.Flag()
	desc := r.Flag()
	var mc uint32
	if desc {
		r.U(8) // colour_primaries
		r.U(8) // transfer_characteristics
		mc = r.U(8)
	}
	if r.Err() != nil {
		return
	}
	s.VideoFullRange = full
	s.ColourDescription = desc
	s.MatrixCoefficients = uint8(mc)
}

// ChromaArrayType is a derived parameter: it is 0 when colour
// planes are separate, otherwise it equals chroma_format_idc.
func (s *SPS) ChromaArrayType() uint32 {
	if s.SeparateColourPlane {
		return 0
	}
	return s.ChromaFormatIDC
}

// chromaSub returns SubWidthC, SubHeightC for the chroma format.
func chromaSub(idc uint32) (subW, subH int) {
	switch idc {
	case 1: // 4:2:0
		return 2, 2
	case 2: // 4:2:2
		return 2, 1
	default: // 3 = 4:4:4
		return 1, 1
	}
}

// computeDimensions computes width/height in pixels accounting for cropping.
func (s *SPS) computeDimensions() error {
	codedW := int(s.WidthMbs) * 16
	codedH := int(s.HeightMapUnits) * 16
	frameMbsOnly := 1
	if !s.FrameMbsOnly {
		frameMbsOnly = 0
		codedH *= 2 // fields: the map-units height is doubled
	}
	if codedW <= 0 || codedH <= 0 || codedW > maxCodedDimension || codedH > maxCodedDimension {
		return fmt.Errorf("h264 sps: coded size %dx%d out of range", codedW, codedH)
	}

	var cropUnitX, cropUnitY int
	if s.ChromaFormatIDC == 0 { // monochrome: ChromaArrayType=0
		cropUnitX = 1
		cropUnitY = 2 - frameMbsOnly
	} else {
		subW, subH := chromaSub(s.ChromaFormatIDC)
		cropUnitX = subW
		cropUnitY = subH * (2 - frameMbsOnly)
	}

	cropXu := (uint64(s.CropLeft) + uint64(s.CropRight)) * uint64(cropUnitX)
	cropYu := (uint64(s.CropTop) + uint64(s.CropBottom)) * uint64(cropUnitY)
	if cropXu >= uint64(codedW) || cropYu >= uint64(codedH) {
		return fmt.Errorf("h264 sps: crop offsets remove entire %dx%d picture", codedW, codedH)
	}
	cropX, cropY := int(cropXu), int(cropYu)
	width := codedW - cropX
	height := codedH - cropY
	if width <= 0 || height <= 0 {
		return fmt.Errorf("h264 sps: cropped size %dx%d is not positive", width, height)
	}
	if width > maxCodedDimension || height > maxCodedDimension {
		return fmt.Errorf("h264 sps: cropped size %dx%d out of range", width, height)
	}
	s.Width = width
	s.Height = height
	offX := uint64(s.CropLeft) * uint64(cropUnitX)
	offY := uint64(s.CropTop) * uint64(cropUnitY)
	if offX > uint64(codedW) || offY > uint64(codedH) {
		return fmt.Errorf("h264 sps: crop origin out of coded %dx%d picture", codedW, codedH)
	}
	s.OffsetX = int(offX)
	s.OffsetY = int(offY)
	return nil
}

// skipScalingList skips over a scaling_list of the given size.
// We don't need the values yet — only to advance the bitstream position correctly.
func skipScalingList(r *bitstream.Reader, size int) {
	lastScale := 8
	nextScale := 8
	for j := 0; j < size; j++ {
		if nextScale != 0 {
			delta := r.SE()
			nextScale = (lastScale + int(delta) + 256) % 256
		}
		if nextScale != 0 {
			lastScale = nextScale
		}
	}
}
