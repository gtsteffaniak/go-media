// Package h264 provides H.264 decode kernels: 4×4/8×8 IDCT residual, dequant, 4×4
// prediction+residual write, and intra prediction helpers.
//
// Every kernel has a portable Go implementation, which is the bit-exact reference. SIMD
// kernels are added per kernel behind KernelsUseSIMD (GOEXPERIMENT=simd, a CPU with
// AVX2/ASIMD, and GOMEDIA_ENABLE_SIMD=1 or -tags gomedia_simd); -tags purego forces portable
// only. A SIMD kernel must be bit-identical to its portable version on shared tests and
// must beat it in benchmarks before it is enabled.
package h264
