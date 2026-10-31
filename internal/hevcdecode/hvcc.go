package hevcdecode

import (
	"encoding/binary"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// HVCCRecord is a parsed HEVCDecoderConfigurationRecord (ISO/IEC 14496-15).
type HVCCRecord struct {
	LengthSize int
	ParamSets  [][]byte
}

// ParseHVCCRecord parses an hvcC box payload.
func ParseHVCCRecord(b []byte) (*HVCCRecord, error) {
	if len(b) < 23 || b[0] != 1 {
		return nil, errs.UnsupProfile("hevc", "invalid or missing hvcC")
	}
	lengthSize := int(b[21]&3) + 1
	if lengthSize < 1 || lengthSize > 4 {
		return nil, errs.UnsupProfile("hevc", "invalid hvcC NAL length size")
	}
	arrays := int(b[22])
	off := 23
	var paramSets [][]byte
	for a := 0; a < arrays && off+3 <= len(b); a++ {
		off++ // array_completeness + NAL type
		n := int(binary.BigEndian.Uint16(b[off : off+2]))
		off += 2
		for i := 0; i < n && off+2 <= len(b); i++ {
			nalLen := int(binary.BigEndian.Uint16(b[off : off+2]))
			off += 2
			if nalLen <= 0 || off+nalLen > len(b) {
				return nil, errs.UnsupProfile("hevc", "truncated hvcC parameter set")
			}
			nal := make([]byte, nalLen)
			copy(nal, b[off:off+nalLen])
			paramSets = append(paramSets, nal)
			off += nalLen
		}
	}
	if len(paramSets) == 0 {
		return nil, errs.UnsupProfile("hevc", "hvcC has no VPS/SPS/PPS")
	}
	return &HVCCRecord{LengthSize: lengthSize, ParamSets: paramSets}, nil
}
