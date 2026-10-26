// Package kernels provides hot numeric paths for thumbnail decode/scale/color.
//
// Every kernel has a portable Go implementation that is the bit-exact reference. SIMD
// variants (GOEXPERIMENT=simd, simd/archsimd) are added per kernel only when they beat the
// portable version; see internal/kernels/h264 for the runtime opt-in gate. Build with
// -tags purego to force portable Go.
package kernels
