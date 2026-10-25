package h264

// Per-kernel SIMD enable flags (compile-time defaults). Production keeps all false;
// enable only after benchmarks prove a win vs purego+PGO (see docs/SIMD_FOLLOWUP_PLAN.md).
var simdKernelEnabled = map[string]bool{
	"idct4x4":    false,
	"idct8x8":    false,
	"dequant4x4": false,
	"recon4x4":   false,
	"intra4x4":   false,
	"intra16x16": false,
	"ycbcr_row":  false,
}

// KernelSIMD reports whether a named kernel may use archsimd at runtime.
func KernelSIMD(name string) bool {
	if !simdExperimentEnabled() || !cpuHasSIMD() || !simdExplicitlyEnabled() {
		return false
	}
	return simdKernelEnabled[name]
}
