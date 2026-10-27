package media

import h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"

// SIMDActive reports whether this binary will dispatch any H.264/YCbCr SIMD kernels.
// Requires GOEXPERIMENT=simd at build time and at least one enabled kernel flag (see docs/SIMD.md).
func SIMDActive() bool {
	return h264k.SIMDActive()
}
