package h264

import (
	"os"
	"strings"

	"golang.org/x/sys/cpu"
)

// KernelsUseSIMD is true only when GOEXPERIMENT=simd code is linked, the CPU has AVX2/ASIMD,
// and SIMD is explicitly enabled (GOMEDIA_ENABLE_SIMD=1 or -tags gomedia_simd). It is resolved
// once at init; kernels must not re-read the environment or CPU flags per call.
//
// No SIMD kernels are implemented yet, so this gate currently selects nothing. Each future
// kernel dispatches on it (or on its own package-level flag derived from it) with a plain
// bool read.
var KernelsUseSIMD = resolveKernelsUseSIMD()

func resolveKernelsUseSIMD() bool {
	return simdExperimentEnabled() && cpuHasSIMD() && simdExplicitlyEnabled()
}

func cpuHasSIMD() bool {
	return cpu.X86.HasAVX2 || cpu.ARM64.HasASIMD
}

// simdExplicitlyEnabled reports whether runtime SIMD dispatch is allowed.
func simdExplicitlyEnabled() bool {
	if v := os.Getenv("GOMEDIA_ENABLE_SIMD"); v != "" {
		return v == "1" || strings.EqualFold(v, "true")
	}
	return simdOptInByDefault()
}
