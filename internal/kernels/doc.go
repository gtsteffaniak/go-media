// Package kernels provides hot numeric paths for thumbnail decode/scale/color.
//
// Production builds use architecture-specific Go assembly on amd64 and arm64
// when CGO_ENABLED=0. Build with -tags purego to force portable Go fallbacks.
//
// Optional faster paths are available when building with GOEXPERIMENT=simd
// (see ycbcr_simd.go); those APIs are experimental and not required at runtime.
package kernels
