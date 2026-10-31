package h264

import (
	"os"
	"testing"
)

func TestSIMDConsumerExperimentKernelsOn(t *testing.T) {
	if !simdExperimentEnabled() {
		t.Skip("requires GOEXPERIMENT=simd build")
	}
	ResetSIMDTestHooks()
	os.Unsetenv("GOMEDIA_DISABLE_SIMD")
	SetCPUHasSIMDForTest(true)
	refreshKernelsUseSIMD()
	if !KernelsUseSIMD {
		t.Fatal("expected KernelsUseSIMD true when experiment + CPU hook")
	}
	if SIMDActive() != anyKernelSIMDEnabled() {
		t.Fatal("SIMDActive must follow the static per-kernel enables")
	}
	ResetSIMDTestHooks()
}

func TestSIMDDisableEnv(t *testing.T) {
	if !simdExperimentEnabled() {
		t.Skip("requires GOEXPERIMENT=simd build")
	}
	ResetSIMDTestHooks()
	SetCPUHasSIMDForTest(true)
	t.Setenv("GOMEDIA_DISABLE_SIMD", "1")
	refreshKernelsUseSIMD()
	if KernelsUseSIMD {
		t.Fatal("expected KernelsUseSIMD false with GOMEDIA_DISABLE_SIMD=1")
	}
	os.Unsetenv("GOMEDIA_DISABLE_SIMD")
	refreshKernelsUseSIMD()
}

func TestSIMDCPUFeatureHook(t *testing.T) {
	if !simdExperimentEnabled() {
		t.Skip("requires GOEXPERIMENT=simd build")
	}
	ResetSIMDTestHooks()
	SetCPUHasSIMDForTest(false)
	refreshKernelsUseSIMD()
	if KernelsUseSIMD {
		t.Fatal("expected false when CPU hook reports no SIMD")
	}
	SetCPUHasSIMDForTest(true)
	refreshKernelsUseSIMD()
	if !KernelsUseSIMD {
		t.Fatal("expected true when CPU hook reports SIMD")
	}
	ResetSIMDTestHooks()
}
