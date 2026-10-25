package h264

import (
	"fmt"

	"github.com/Eyevinn/mp4ff/avc"
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// H.264 Baseline (profile_idc 66) uses CAVLC. Supported only when the clean-room
// SIMD-first path passes the ≤20% ffmpeg thumbnail gate (see docs/BASELINE_PROFILE.md).
const h264ProfileBaseline = 66

// validateSPSNAL rejects profiles go-media does not decode for thumbnails.
func validateSPSNAL(nal []byte) error {
	sps, err := avc.ParseSPSNALUnit(nal, false)
	if err != nil {
		return errs.UnsupProfile("h264", fmt.Sprintf("sps: %v", err))
	}
	if sps.Profile == h264ProfileBaseline {
		return errs.UnsupProfile("h264", "baseline profile (CAVLC): unsupported for performance; use ffmpeg")
	}
	if sps.ChromaFormatIDC != 1 {
		return errs.UnsupProfile("h264", fmt.Sprintf("chroma format %d (need 4:2:0)", sps.ChromaFormatIDC))
	}
	if sps.BitDepthLumaMinus8 != 0 || sps.BitDepthChromaMinus8 != 0 {
		return errs.UnsupProfile("h264", "10-bit or deeper luma/chroma")
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
