//go:build goexperiment.simd && !purego

package kernels

import "simd"

// AreaDownRowYSIMD is an optional GOEXPERIMENT=simd path for luma averaging.
// It must produce identical results to AreaDownRowY.
func AreaDownRowYSIMD(dstY []byte, dx int, srcY []byte, srcStride int, sy0, sy1, x0, x1 int) {
	// Portable simd emulation is correct but not always faster on small tiles;
	// delegate to the production path until tile sizes justify vector loops.
	AreaDownRowY(dstY, 0, dx, srcY, srcStride, sy0, sy1, x0, x1)
	_ = simd.Int8s{}
}
