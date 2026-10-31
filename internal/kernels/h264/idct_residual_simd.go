//go:build goexperiment.simd

package h264

import "simd"

// inverseTransform4x4ResidualSIMD applies H.264 4×4 IDCT using simd.Int32s for rounding rows.
func inverseTransform4x4ResidualSIMD(d [16]int32) [16]int32 {
	var f [16]int32
	for i := 0; i < 4; i++ {
		butterfly1DInt32s(d[i*4:], f[i*4:])
	}
	var h [16]int32
	for j := 0; j < 4; j++ {
		var col [4]int32
		for i := 0; i < 4; i++ {
			col[i] = f[i*4+j]
		}
		var out [4]int32
		butterfly1DInt32s(col[:], out[:])
		for i := 0; i < 4; i++ {
			h[i*4+j] = out[i]
		}
	}
	var r [16]int32
	thirtyTwo := simd.BroadcastInt32s(32)
	for i := 0; i < 4; i++ {
		row, _ := simd.LoadInt32sPart(h[i*4 : i*4+4])
		rounded := row.Add(thirtyTwo).ShiftAllRight(6)
		rounded.StorePart(r[i*4 : i*4+4])
	}
	return r
}

func butterfly1DInt32s(in []int32, out []int32) {
	a0, a1, a2, a3 := in[0], in[1], in[2], in[3]
	e0 := a0 + a2
	e1 := a0 - a2
	e2 := (a1 >> 1) - a3
	e3 := a1 + (a3 >> 1)
	var res [4]int32
	res[0] = e0 + e3
	res[1] = e1 + e2
	res[2] = e1 - e2
	res[3] = e0 - e3
	v, _ := simd.LoadInt32sPart(res[:])
	v.StorePart(out)
}
