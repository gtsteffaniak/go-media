package decode

import "fmt"

// ProfileName maps profile_idc to the familiar H.264 profile name.
func ProfileName(idc uint8) string {
	switch idc {
	case 66:
		return "Baseline"
	case 77:
		return "Main"
	case 88:
		return "Extended"
	case 100:
		return "High"
	case 110:
		return "High 10"
	case 122:
		return "High 4:2:2"
	case 244:
		return "High 4:4:4 Predictive"
	default:
		return fmt.Sprintf("profile_idc=%d", idc)
	}
}

// ChromaName maps chroma_format_idc to the subsampling name.
func ChromaName(idc uint32) string {
	switch idc {
	case 0:
		return "monochrome"
	case 1:
		return "4:2:0"
	case 2:
		return "4:2:2"
	case 3:
		return "4:4:4"
	default:
		return fmt.Sprintf("chroma_format_idc=%d", idc)
	}
}

// Summary is a short string for logs/diagnostics.
func (s *SPS) Summary() string {
	return fmt.Sprintf("%s, %dx%d, %s, %d-bit (level %.1f)",
		ProfileName(s.ProfileIDC), s.Width, s.Height,
		ChromaName(s.ChromaFormatIDC), s.BitDepthLuma, float64(s.LevelIDC)/10)
}
