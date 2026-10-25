package decode

// 8x8 transform path for High profile (ISO/IEC 14496-10): zig-zag scan,
// dequantization and the inverse 8x8 integer transform. Flat scaling matrix
// (weightScale = 16); custom scaling lists are not yet supported.

// zigzag8x8 — 8x8 frame zig-zag scan: scan position → raster index (row*8 + col).
var zigzag8x8 = [64]int{
	0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5,
	12, 19, 26, 33, 40, 48, 41, 34, 27, 20, 13, 6, 7, 14, 21, 28,
	35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
	58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
}

// normAdjust8x8 (the "v" table for 8x8), rows m = QP%6, 6 position classes.
var normAdjust8x8 = [6][6]int32{
	{20, 18, 32, 19, 25, 24},
	{22, 19, 35, 21, 28, 26},
	{26, 23, 42, 24, 33, 31},
	{28, 25, 45, 26, 35, 33},
	{32, 28, 51, 30, 40, 38},
	{36, 32, 58, 34, 46, 43},
}

// class8x8 maps an 8x8 position to its normAdjust class (0..5).
func class8x8(i, j int) int {
	i4, j4 := i%4, j%4
	switch {
	case i4 == 0 && j4 == 0:
		return 0
	case i%2 == 1 && j%2 == 1:
		return 1
	case i4 == 2 && j4 == 2:
		return 2
	case (i4 == 0 && j%2 == 1) || (i%2 == 1 && j4 == 0):
		return 3
	case (i4 == 0 && j4 == 2) || (i4 == 2 && j4 == 0):
		return 4
	default:
		return 5
	}
}

// dequant8x8 scales the 64 coefficients of an 8x8 block (row-major), flat matrix.
func dequant8x8(coef [64]int32, qp int) [64]int32 {
	m := qp % 6
	shift := qp / 6
	var d [64]int32
	for idx := 0; idx < 64; idx++ {
		ls := 16 * normAdjust8x8[m][class8x8(idx/8, idx%8)]
		c := coef[idx]
		if shift >= 6 {
			d[idx] = (c * ls) << uint(shift-6)
		} else {
			d[idx] = (c*ls + (1 << uint(5-shift))) >> uint(6-shift)
		}
	}
	return d
}

// inverseScan8x8 lays out 64 scan-order coefficients into raster order.
func inverseScan8x8(scan []int32) [64]int32 {
	var block [64]int32
	for s := 0; s < 64 && s < len(scan); s++ {
		block[zigzag8x8[s]] = scan[s]
	}
	return block
}

// idct8 — the 1D inverse 8x8 integer transform of an 8-element vector.
func idct8(g [8]int32) [8]int32 {
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

// inverseTransform8x8 applies the inverse transform to rows then columns and
// returns the residual r = (e + 32) >> 6.
func inverseTransform8x8(d [64]int32) [64]int32 {
	var f [64]int32
	for r := 0; r < 8; r++ {
		var g [8]int32
		copy(g[:], d[r*8:r*8+8])
		m := idct8(g)
		copy(f[r*8:r*8+8], m[:])
	}
	var h [64]int32
	for c := 0; c < 8; c++ {
		var g [8]int32
		for r := 0; r < 8; r++ {
			g[r] = f[r*8+c]
		}
		m := idct8(g)
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
