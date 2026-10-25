// Package h264 parses H.264 syntax on top of bitstream/nal. So far it covers
// the parameter sets (SPS/PPS), enough for frame dimensions and to determine
// the profile/entropy coding. Decoding the macroblocks themselves is for later
// phases.
package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

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

	if isHighProfile(s.ProfileIDC) {
		s.ChromaFormatIDC = r.UE()
		if s.ChromaFormatIDC == 3 {
			s.SeparateColourPlane = r.Flag()
		}
		s.BitDepthLuma = r.UE() + 8
		s.BitDepthChroma = r.UE() + 8
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

	s.Log2MaxFrameNum = int(r.UE()) + 4 // log2_max_frame_num_minus4 + 4
	s.PicOrderCntType = r.UE()
	switch s.PicOrderCntType {
	case 0:
		s.Log2MaxPocLsb = int(r.UE()) + 4 // log2_max_pic_order_cnt_lsb_minus4 + 4
	case 1:
		s.DeltaPicOrderAlwaysZero = r.Flag()
		r.SE() // offset_for_non_ref_pic
		r.SE() // offset_for_top_to_bottom_field
		num := r.UE()
		for i := uint32(0); i < num; i++ {
			r.SE() // offset_for_ref_frame[i]
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
	// vui_parameters_present_flag and the VUI are not needed for dimensions — skip them.

	if err := r.Err(); err != nil {
		return nil, err
	}
	s.computeDimensions()
	return s, nil
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
func (s *SPS) computeDimensions() {
	width := int(s.WidthMbs) * 16
	height := int(s.HeightMapUnits) * 16
	frameMbsOnly := 1
	if !s.FrameMbsOnly {
		frameMbsOnly = 0
		height *= 2 // fields: the map-units height is doubled
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

	width -= int(s.CropLeft+s.CropRight) * cropUnitX
	height -= int(s.CropTop+s.CropBottom) * cropUnitY
	s.Width = width
	s.Height = height
	s.OffsetX = int(s.CropLeft) * cropUnitX // luma pixel origin of the visible region
	s.OffsetY = int(s.CropTop) * cropUnitY
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
