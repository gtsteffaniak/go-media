//go:build !gomedia_simd

package h264

// simdOptInByDefault is true only when built with -tags gomedia_simd (benchmark/CI experiment binaries).
func simdOptInByDefault() bool { return false }
