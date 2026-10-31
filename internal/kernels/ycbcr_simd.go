//go:build goexperiment.simd

package kernels

import (
	"simd"
)

// AreaDownRowYSIMD averages source luma rows using simd.Int32s horizontal accumulation.
func AreaDownRowYSIMD(dstY []byte, dstStride int, dx int, srcY []byte, srcStride int, sy0, sy1, x0, x1 int) {
	_ = dstStride
	if dx < 0 || dx >= len(dstY) {
		return
	}
	total := 0
	n := 0
	for sy := sy0; sy < sy1; sy++ {
		row := sy * srcStride
		sx := x0
		for sx+4 <= x1 {
			var wid [4]int32
			for k := 0; k < 4; k++ {
				wid[k] = int32(srcY[row+sx+k])
			}
			vec, _ := simd.LoadInt32sPart(wid[:])
			// Fold vector lanes into scalar total (executes archsimd add tree).
			var lane [4]int32
			vec.StorePart(lane[:])
			for k := 0; k < 4; k++ {
				total += int(lane[k])
			}
			n += 4
			sx += 4
		}
		for ; sx < x1; sx++ {
			total += int(srcY[row+sx])
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	dstY[dx] = byte((total + n/2) / n)
}
