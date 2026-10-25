package h264

import (
	"os"
	"strings"

	"golang.org/x/sys/cpu"
)

// KernelsUseSIMD is true only when GOEXPERIMENT=simd code is linked, the CPU has AVX2/ASIMD,
// and SIMD is explicitly enabled (GOMEDIA_ENABLE_SIMD=1 or -tags gomedia_simd). Production
// builds keep this false even under GOEXPERIMENT=simd.
var KernelsUseSIMD bool

func init() {
	KernelsUseSIMD = simdExperimentEnabled() && cpuHasSIMD() && simdExplicitlyEnabled()
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

// SetSIMDEnabledForTest toggles SIMD dispatch (tests only).
func SetSIMDEnabledForTest(on bool) {
	if on {
		os.Setenv("GOMEDIA_ENABLE_SIMD", "1")
	} else {
		os.Setenv("GOMEDIA_ENABLE_SIMD", "0")
	}
	KernelsUseSIMD = simdExperimentEnabled() && cpuHasSIMD() && simdExplicitlyEnabled()
}
