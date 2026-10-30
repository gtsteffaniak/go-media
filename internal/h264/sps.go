package h264

import (
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"
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
	sps, err := avc.ParseSPSNALUnit(nal, false)
	if err != nil {
		return errs.UnsupProfile("h264", fmt.Sprintf("sps: %v", err))
	}
	if reason, gated := gatedProfiles[int(sps.Profile)]; gated && !allowGatedProfiles.Load() {
		return errs.UnsupProfile("h264", reason)
	}
	if sps.ChromaFormatIDC != 1 {
		return errs.UnsupProfile("h264", fmt.Sprintf("chroma format %d (need 4:2:0)", sps.ChromaFormatIDC))
	}
	if sps.BitDepthLumaMinus8 > 2 || sps.BitDepthChromaMinus8 > 2 {
		// 9/10-bit 4:2:0 is reconstructed on a scalar uint16 path; deeper
		// samples (High 4:4:4 Predictive, 12/14-bit) are not.
		return errs.UnsupProfile("h264", fmt.Sprintf("%d-bit luma / %d-bit chroma (profile_idc %d); decoder supports 8, 9 and 10 bits",
			sps.BitDepthLumaMinus8+8, sps.BitDepthChromaMinus8+8, sps.Profile))
	}
	if sps.SeqScalingMatrixPresentFlag {
		// High profile 8x8 transform is supported when lists are present.
	}
	if sps.MbAdaptiveFrameFieldFlag {
		return errs.UnsupProfile("h264", "MBAFF")
	}
	if sps.FrameMbsOnlyFlag == false {
		return errs.UnsupProfile("h264", "interlaced frame encoding")
	}
	return nil
}
