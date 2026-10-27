// Package h264 provides SIMD-oriented H.264 decode kernels: 4×4 IDCT residual,
// 4×4 prediction+residual write, and future intra/8×8 helpers.
//
// Default builds use portable Go for H.264 DSP. GOEXPERIMENT=simd links optional
// experiment SIMD bodies (simd_kernel_flags.go; docs/SIMD.md). All paths must be
// bit-identical on shared tests.
package h264
