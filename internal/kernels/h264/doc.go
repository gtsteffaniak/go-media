// Package h264 provides SIMD-oriented H.264 decode kernels: 4×4 IDCT residual,
// 4×4 prediction+residual write, and future intra/8×8 helpers.
//
// Dispatch: portable Go by default. GOEXPERIMENT=simd links archsimd kernels, but runtime
// SIMD is off unless GOMEDIA_ENABLE_SIMD=1 or -tags gomedia_simd (experiments only).
// amd64 production builds may use assembly-backed paths. -tags purego forces portable
// only. All paths must be bit-identical on shared tests.
package h264
