package decode

import "fmt"

// MBKind is the macroblock category of an I-slice.
type MBKind int

const (
	MbINxN   MBKind = iota // I_NxN: intra 4x4 (or 8x8) prediction
	MbI16x16               // I_16x16: intra 16x16 prediction
	MbIPCM                 // I_PCM: raw samples without prediction/transform
)

func (k MBKind) String() string {
	switch k {
	case MbINxN:
		return "I_NxN"
	case MbI16x16:
		return "I_16x16"
	default:
		return "I_PCM"
	}
}

// MBType is the parsed mb_type of an I-slice macroblock.
type MBType struct {
	Kind               MBKind
	Intra16x16PredMode int // 0..3 (only for I_16x16)
	CBPChroma          int // coded_block_pattern for chroma: 0/1/2
	CBPLuma            int // 0 or 15 (for I_16x16)
}

// decodeIMBType parses an I-slice mb_type.
// mb_type 0 = I_NxN; 1..24 = I_16x16 (with mode and CBP layout); 25 = I_PCM.
func decodeIMBType(mbType uint32) (MBType, error) {
	switch {
	case mbType == 0:
		return MBType{Kind: MbINxN}, nil
	case mbType >= 1 && mbType <= 24:
		n := int(mbType) - 1
		cbpLuma := 0
		if n/12 != 0 {
			cbpLuma = 15
		}
		return MBType{
			Kind:               MbI16x16,
			Intra16x16PredMode: n % 4,
			CBPChroma:          (n / 4) % 3,
			CBPLuma:            cbpLuma,
		}, nil
	case mbType == 25:
		return MBType{Kind: MbIPCM}, nil
	default:
		return MBType{}, fmt.Errorf("h264: invalid mb_type=%d in I-slice", mbType)
	}
}
