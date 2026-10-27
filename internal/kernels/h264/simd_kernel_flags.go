package h264

// Per-kernel SIMD dispatch when KernelsUseSIMD is true (GOEXPERIMENT=simd + CPU features).
var simdKernelEnabled = map[string]bool{
	"idct4x4":    true,
	"idct8x8":    true,
	"dequant4x4": true,
	"recon4x4":   true,
	"intra4x4":   true,
	"intra16x16": true,
	"idct_add16": true,
	"ycbcr_row":  false, // off until SIMD matches portable rounding
}

// KernelSIMD reports whether a named kernel uses the experiment SIMD implementation.
func KernelSIMD(name string) bool {
	if !KernelsUseSIMD {
		return false
	}
	return simdKernelEnabled[name]
}

func anyKernelSIMDEnabled() bool {
	for _, on := range simdKernelEnabled {
		if on {
			return true
		}
	}
	return false
}
