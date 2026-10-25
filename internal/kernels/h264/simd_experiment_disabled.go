//go:build !goexperiment.simd

package h264

func simdExperimentEnabled() bool { return false }

// SIMDExperimentEnabled reports whether the binary was built with GOEXPERIMENT=simd.
func SIMDExperimentEnabled() bool { return false }
