package h264

import (
	"os"
	"strings"

	"golang.org/x/sys/cpu"
)

// KernelsUseSIMD is true when GOEXPERIMENT=simd code is linked, the CPU has AVX2/ASIMD,
// and SIMD is not disabled via GOMEDIA_DISABLE_SIMD.
var KernelsUseSIMD bool

// testSIMDForce enables or disables SIMD dispatch in tests (overrides disable env).
var testSIMDForce *bool

// forceCPUHasSIMD overrides cpuHasSIMD for tests when non-nil.
var forceCPUHasSIMD *bool

func init() {
	refreshKernelsUseSIMD()
}

func refreshKernelsUseSIMD() {
	if testSIMDForce != nil {
		KernelsUseSIMD = *testSIMDForce && simdExperimentEnabled() && cpuHasSIMD()
		return
	}
	if simdDisabledByEnv() {
		KernelsUseSIMD = false
		return
	}
	KernelsUseSIMD = simdExperimentEnabled() && cpuHasSIMD()
}

func simdDisabledByEnv() bool {
	v := os.Getenv("GOMEDIA_DISABLE_SIMD")
	return v == "1" || strings.EqualFold(v, "true")
}

func cpuHasSIMD() bool {
	if forceCPUHasSIMD != nil {
		return *forceCPUHasSIMD
	}
	return cpu.X86.HasAVX2 || cpu.ARM64.HasASIMD
}

// SetSIMDEnabledForTest toggles experiment routing (tests only).
func SetSIMDEnabledForTest(on bool) {
	testSIMDForce = &on
	refreshKernelsUseSIMD()
}

// SetCPUHasSIMDForTest forces the CPU feature probe result (tests only).
func SetCPUHasSIMDForTest(has bool) {
	forceCPUHasSIMD = &has
	refreshKernelsUseSIMD()
}

// ResetSIMDTestHooks clears test overrides and restores default kernel flags (all off).
func ResetSIMDTestHooks() {
	testSIMDForce = nil
	forceCPUHasSIMD = nil
	for name := range simdKernelEnabled {
		simdKernelEnabled[name] = name != "ycbcr_row"
	}
	refreshKernelsUseSIMD()
}
