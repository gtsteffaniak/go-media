package decode

import (
	"fmt"
	"math"
)

// defaultMaxFrameMacroblocks is H.264 level 6.2 MaxFS (macroblocks per frame).
const defaultMaxFrameMacroblocks = 139264

// MaxFrameMacroblocks caps WidthMbs×FrameHeightInMbs when parsing SPS.
// Zero means defaultMaxFrameMacroblocks (level 6.2).
var MaxFrameMacroblocks uint64

func maxFrameMacroblocks() uint64 {
	if MaxFrameMacroblocks == 0 {
		return defaultMaxFrameMacroblocks
	}
	return MaxFrameMacroblocks
}

const (
	maxSeqParameterSetID              = 31
	maxLog2MaxFrameNumMinus4          = 12
	maxLog2MaxPicOrderCntLsbMinus4     = 12
	maxNumRefFramesInPicOrderCntCycle = 255
	maxChromaFormatIDC                = 3
	maxBitDepth                       = 14
	maxParameterSetID                 = 255
	maxCodedDimension                 = 65535 // pixels per axis for allocations
)

func frameHeightInMbs(s *SPS) (uint64, error) {
	if s.HeightMapUnits == 0 {
		return 0, fmt.Errorf("h264 sps: pic_height_in_map_units is zero")
	}
	if s.FrameMbsOnly {
		return uint64(s.HeightMapUnits), nil
	}
	if s.HeightMapUnits > uint32(math.MaxUint32/2) {
		return 0, fmt.Errorf("h264 sps: pic_height_in_map_units overflows frame height")
	}
	return uint64(s.HeightMapUnits) * 2, nil
}

func macroblockCount(s *SPS) (uint64, error) {
	if s.WidthMbs == 0 {
		return 0, fmt.Errorf("h264 sps: pic_width_in_mbs is zero")
	}
	h, err := frameHeightInMbs(s)
	if err != nil {
		return 0, err
	}
	if uint64(s.WidthMbs) > math.MaxUint64/h {
		return 0, fmt.Errorf("h264 sps: frame macroblock count overflows")
	}
	return uint64(s.WidthMbs) * h, nil
}

func checkMacroblockBudget(s *SPS) error {
	mbs, err := macroblockCount(s)
	if err != nil {
		return err
	}
	limit := maxFrameMacroblocks()
	if mbs > limit {
		return fmt.Errorf("h264 sps: %d macroblocks exceeds limit %d", mbs, limit)
	}
	return nil
}
