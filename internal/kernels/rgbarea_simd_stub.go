//go:build !(goexperiment.simd && amd64 && !purego)

package kernels

import "github.com/gtsteffaniak/go-media/internal/h264/yuv"

const simdRGBArea = false

func rgbAccumRow420SIMD(_, _, _ []int32, _, _, _ []byte, _ *yuv.RGBLUT) int { return 0 }
