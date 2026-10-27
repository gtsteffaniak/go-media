package h264

// InverseTransform8x8Residual applies the H.264 8×8 inverse integer transform with
// (coeff + 32) >> 6 rounding. Coefficients are dequantized int32 values in raster order.
func InverseTransform8x8Residual(d [64]int32) [64]int32 {
	return inverseTransform8x8Residual(d)
}

func inverseTransform8x8ResidualPortable(d [64]int32) [64]int32 {
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
	for k := 0; k < 64; k++ {
		res[k] = (h[k] + 32) >> 6
	}
	return res
}

func idct8Portable(g []int32) [8]int32 {
	var a [8]int32
	a[0] = g[0] + g[4]
	a[2] = g[0] - g[4]
	a[4] = (g[2] >> 1) - g[6]
	a[6] = g[2] + (g[6] >> 1)
	a[1] = -g[3] + g[5] - g[7] - (g[7] >> 1)
	a[3] = g[1] + g[7] - g[3] - (g[3] >> 1)
	a[5] = -g[1] + g[7] + g[5] + (g[5] >> 1)
	a[7] = g[3] + g[5] + g[1] + (g[1] >> 1)
	var b [8]int32
	b[0] = a[0] + a[6]
	b[2] = a[2] + a[4]
	b[4] = a[2] - a[4]
	b[6] = a[0] - a[6]
	b[1] = a[1] + (a[7] >> 2)
	b[3] = a[3] + (a[5] >> 2)
	b[5] = (a[3] >> 2) - a[5]
	b[7] = a[7] - (a[1] >> 2)
	var m [8]int32
	m[0] = b[0] + b[7]
	m[1] = b[2] + b[5]
	m[2] = b[4] + b[3]
	m[3] = b[6] + b[1]
	m[4] = b[6] - b[1]
	m[5] = b[4] - b[3]
	m[6] = b[2] - b[5]
	m[7] = b[0] - b[7]
	return m
}
