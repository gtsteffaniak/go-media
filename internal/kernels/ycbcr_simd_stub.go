//go:build !goexperiment.simd || purego

package kernels

// AreaDownRowYSIMD is a stub when GOEXPERIMENT=simd is off.
func AreaDownRowYSIMD(dstY []byte, dstStride int, dx int, srcY []byte, srcStride int, sy0, sy1, x0, x1 int) {
	_ = dstStride
	areaDownRowYPortable(dstY, dx, srcY, srcStride, sy0, sy1, x0, x1)
}
