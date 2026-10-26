//go:build gomedia_simd

package h264

// Experiment tag build: per-kernel flags remain false until a benchmark proves a win.
// Tests enable kernels via SetKernelSIMDForTest + GOMEDIA_ENABLE_SIMD=1.
