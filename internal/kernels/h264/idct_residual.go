package h264

// InverseTransform4x4Residual applies the H.264 4×4 inverse integer transform with
// the standard (coeff + 32) >> 6 rounding. Input coefficients are dequantized
// int32 values in raster order; output is the 4×4 residual block (raster order).
func InverseTransform4x4Residual(d [16]int32) [16]int32 {
	return inverseTransform4x4ResidualPortable(d)
}

func inverseTransform4x4ResidualPortable(d [16]int32) [16]int32 {
	var f [16]int32
	for i := 0; i < 4; i++ {
		o := i * 4
		e0 := d[o+0] + d[o+2]
		e1 := d[o+0] - d[o+2]
		e2 := (d[o+1] >> 1) - d[o+3]
		e3 := d[o+1] + (d[o+3] >> 1)
		f[o+0] = e0 + e3
		f[o+1] = e1 + e2
		f[o+2] = e1 - e2
		f[o+3] = e0 - e3
	}
	var h [16]int32
	for j := 0; j < 4; j++ {
		g0 := f[0*4+j] + f[2*4+j]
		g1 := f[0*4+j] - f[2*4+j]
		g2 := (f[1*4+j] >> 1) - f[3*4+j]
		g3 := f[1*4+j] + (f[3*4+j] >> 1)
		h[0*4+j] = g0 + g3
		h[1*4+j] = g1 + g2
		h[2*4+j] = g1 - g2
		h[3*4+j] = g0 - g3
	}
	var r [16]int32
	for k := 0; k < 16; k++ {
		r[k] = (h[k] + 32) >> 6
	}
	return r
}
