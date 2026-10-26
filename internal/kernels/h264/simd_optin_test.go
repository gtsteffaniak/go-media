package h264

import (
	"os"
	"testing"
)

func TestSIMDOffByDefaultWithExperimentBuild(t *testing.T) {
	if !simdExperimentEnabled() {
		t.Skip("requires GOEXPERIMENT=simd build")
	}
	if v, ok := os.LookupEnv("GOMEDIA_ENABLE_SIMD"); ok {
		defer os.Setenv("GOMEDIA_ENABLE_SIMD", v)
	} else {
		defer os.Unsetenv("GOMEDIA_ENABLE_SIMD")
	}
	os.Unsetenv("GOMEDIA_ENABLE_SIMD")
	if simdOptInByDefault() {
		t.Skip("built with -tags gomedia_simd")
	}
	if resolveKernelsUseSIMD() {
		t.Fatal("expected SIMD off without GOMEDIA_ENABLE_SIMD")
	}
}
