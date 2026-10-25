//go:build goexperiment.simd

package h264

// simdExperimentEnabled reports whether this binary was built with GOEXPERIMENT=simd.
func simdExperimentEnabled() bool { return true }

// SIMDExperimentEnabled reports whether the binary was built with GOEXPERIMENT=simd.
func SIMDExperimentEnabled() bool { return true }
