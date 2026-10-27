// Package kernels provides hot numeric paths for thumbnail decode/scale/color.
//
// Production builds use CGO_ENABLED=0 with -pgo=default.pgo (see docs/PROFILING.md).
// On amd64/arm64, YCbCr area
// downscale uses plan9 assembly; other GOARCHes use portable Go. Optional H.264 SIMD
// kernels require GOEXPERIMENT=simd at build time (see docs/SIMD.md).
package kernels
