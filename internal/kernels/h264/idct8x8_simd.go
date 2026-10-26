//go:build goexperiment.simd && !purego

package h264

import "simd"

func inverseTransform8x8ResidualSIMD(d [64]int32) [64]int32 {
	recordSIMD(ProbeIDCT8x8)
	var f [64]int32
	for r := 0; r < 8; r++ {
		m := idct8Portable(d[r*8 : r*8+8])
		copy(f[r*8:r*8+8], m[:])
	}
	var h [64]int32
	for c := 0; c < 8; c++ {
		var col [8]int32
		for r := 0; r < 8; r++ {
			col[r] = f[r*8+c]
		}
		m := idct8Portable(col[:])
		for r := 0; r < 8; r++ {
			h[r*8+c] = m[r]
		}
	}
	var res [64]int32
	thirtyTwo := simd.BroadcastInt32s(32)
	for r := 0; r < 8; r++ {
		row, _ := simd.LoadInt32sPart(h[r*8 : r*8+8])
		rounded := row.Add(thirtyTwo).ShiftAllRight(6)
		rounded.StorePart(res[r*8 : r*8+8])
	}
	return res
}
