// Package h264 provides SIMD-oriented H.264 decode kernels: 4×4 IDCT residual,
// 4×4 prediction+residual write, and future intra/8×8 helpers.
//
// Dispatch: portable Go by default; GOEXPERIMENT=simd uses archsimd hooks when enabled;
// amd64 production builds may use assembly-backed paths. -tags purego forces portable
// only. All paths must be bit-identical on shared tests.
package h264
