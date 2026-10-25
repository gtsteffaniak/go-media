// Package h264 provides SIMD-oriented H.264 decode kernels: dequant, 4×4/8×8 IDCT,
// residual add, intra prediction helpers, and plane writes. Portable, amd64 asm,
// optional GOEXPERIMENT=simd, and -tags purego paths must be bit-identical on tests.
package h264
