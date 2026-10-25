package h264

import (
	"os"
	"testing"
)

func TestSIMDOffByDefaultWithExperimentBuild(t *testing.T) {
	if !simdExperimentEnabled() {
		t.Skip("requires GOEXPERIMENT=simd build")
	}
	os.Unsetenv("GOMEDIA_ENABLE_SIMD")
	KernelsUseSIMD = simdExperimentEnabled() && cpuHasSIMD() && simdExplicitlyEnabled()
	if KernelsUseSIMD {
		t.Fatal("expected KernelsUseSIMD false without GOMEDIA_ENABLE_SIMD")
	}
}
