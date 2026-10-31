package h264

import (
	"os"
	"strings"

	"golang.org/x/sys/cpu"
)

// KernelsUseSIMD is true when GOEXPERIMENT=simd code is linked, the CPU has AVX2/ASIMD,
// and SIMD is not disabled via GOMEDIA_DISABLE_SIMD.
var KernelsUseSIMD bool

// KernelsUseAVX512 additionally selects the AVX-512 tier of kernels that have one
// (amd64 only). It is false whenever KernelsUseSIMD is false, and can be turned off
// on its own with GOMEDIA_DISABLE_AVX512=1 to measure or ship the AVX2 tier.
var KernelsUseAVX512 bool

// forceAVX512 overrides AVX-512 detection for tests when non-nil.
var forceAVX512 *bool

// testSIMDForce enables or disables SIMD dispatch in tests (overrides disable env).
var testSIMDForce *bool

// forceCPUHasSIMD overrides cpuHasSIMD for tests when non-nil.
var forceCPUHasSIMD *bool

func init() {
	refreshKernelsUseSIMD()
}

func refreshKernelsUseSIMD() {
	refreshSIMDBase()
	avx := archHasAVX512()
	if forceAVX512 != nil {
		avx = *forceAVX512 && archHasAVX512()
	} else if envTrue("GOMEDIA_DISABLE_AVX512") {
		avx = false
	}
	KernelsUseAVX512 = KernelsUseSIMD && avx
}

func refreshSIMDBase() {
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

func simdDisabledByEnv() bool { return envTrue("GOMEDIA_DISABLE_SIMD") }

func envTrue(name string) bool {
	v := os.Getenv(name)
	return v == "1" || strings.EqualFold(v, "true")
}

// SetAVX512EnabledForTest toggles the AVX-512 tier (tests / benchmarks only).
func SetAVX512EnabledForTest(on bool) {
	forceAVX512 = &on
	refreshKernelsUseSIMD()
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

// ResetSIMDTestHooks clears test overrides and re-reads the environment.
func ResetSIMDTestHooks() {
	testSIMDForce = nil
	forceCPUHasSIMD = nil
	forceAVX512 = nil
	refreshKernelsUseSIMD()
}
