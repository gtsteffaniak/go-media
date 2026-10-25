// Package kernels provides hot numeric paths for thumbnail decode/scale/color.
//
// Production builds use architecture-specific Go assembly on amd64 and arm64
// when CGO_ENABLED=0. Build with -tags purego to force portable Go fallbacks.
//
// Production builds use -tags purego with default.pgo. GOEXPERIMENT=simd and amd64 asm are opt-in;
// enable runtime SIMD only with GOMEDIA_ENABLE_SIMD=1.
// (see ycbcr_simd.go); those APIs are experimental and not required at runtime.
package kernels
