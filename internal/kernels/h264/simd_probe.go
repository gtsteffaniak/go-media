package h264

import "sync/atomic"

// SIMDProbeCounts records how many times each SIMD kernel entry point ran (tests/benches).
var SIMDProbeCounts atomic.Uint64

// ProbeName identifies a SIMD kernel for testing.
type ProbeName string

const (
	ProbeIDCT4x4    ProbeName = "idct4x4"
	ProbeIDCT8x8    ProbeName = "idct8x8"
	ProbeRecon4x4   ProbeName = "recon4x4"
	ProbeIntra4x4   ProbeName = "intra4x4"
	ProbeDequant4x4 ProbeName = "dequant4x4"
	ProbeYCbCrRow   ProbeName = "ycbcr_row"
)

func recordSIMD(_ ProbeName) {
	SIMDProbeCounts.Add(1)
}

// RecordYCbCrRowSIMD marks a YCbCr downscale SIMD kernel invocation.
func RecordYCbCrRowSIMD() { recordSIMD(ProbeYCbCrRow) }

// ResetSIMDProbe clears the global SIMD execution counter.
func ResetSIMDProbe() { SIMDProbeCounts.Store(0) }

// SIMDProbeHits returns the number of SIMD kernel invocations since last reset.
func SIMDProbeHits() uint64 { return SIMDProbeCounts.Load() }
