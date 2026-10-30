// Package decode implements H.264 slice/MB decode (mgvs/go-openh264 BSD-2; see THIRD_PARTY_NOTICES.md).
// 8-bit samples use byte planes and SIMD kernels; 9/10-bit 4:2:0 streams use
// the scalar uint16 path in hbd*.go and are rounded to 8 bits per slice.
package decode

import h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"

// Inverse dequantization and integer transforms for H.264.
// All functions operate on integer coefficients and do not depend on the
// bitstream — they are easy to verify with deterministic vectors. Flat
// quantization matrix (weightScale = 16); scaling lists (High profile) will be
// added later.

// normAdjust4x4 (the "v" table): rows m = QP%6, columns are the position class.
var normAdjust4x4 = [6][3]int32{
	{10, 16, 13},
	{11, 18, 14},
	{13, 20, 16},
	{14, 23, 18},
	{16, 25, 20},
	{18, 29, 23},
}

// posClass4x4: 0 — both coordinates even, 1 — both odd, 2 — otherwise.
func posClass4x4(i, j int) int {
	iEven := i%2 == 0
	jEven := j%2 == 0
	switch {
	case iEven && jEven:
		return 0
	case !iEven && !jEven:
		return 1
	default:
		return 2
	}
}

// levelScale4x4 with a flat matrix: LevelScale = weightScale(16) · normAdjust.
func levelScale4x4(m, i, j int) int32 {
	return 16 * normAdjust4x4[m][posClass4x4(i, j)]
}

// dequant4x4 scales the 16 coefficients of a 4x4 block (kernel portable/simd dispatch).
func dequant4x4(coef [16]int32, qp int) [16]int32 {
	return h264k.Dequant4x4(coef, qp)
}

// inverseTransform4x4 applies the kernel IDCT + rounding (bit-identical portable/simd/asm dispatch).
func inverseTransform4x4(d [16]int32) [16]int32 {
	return h264k.InverseTransform4x4Residual(d)
}

// inverseLumaDC — inverse 4x4 Hadamard over the I_16x16 DC coefficients and
// their scaling. Result is 16 DC values, one per 4x4 block.
func inverseLumaDC(c [16]int32, qp int) [16]int32 {
	var f [16]int32
	for i := 0; i < 4; i++ {
		o := i * 4
		e0 := c[o+0] + c[o+2]
		e1 := c[o+0] - c[o+2]
		e2 := c[o+1] - c[o+3]
		e3 := c[o+1] + c[o+3]
		f[o+0] = e0 + e3
		f[o+1] = e1 + e2
		f[o+2] = e1 - e2
		f[o+3] = e0 - e3
	}
	var g [16]int32
	for j := 0; j < 4; j++ {
		a0 := f[0*4+j] + f[2*4+j]
		a1 := f[0*4+j] - f[2*4+j]
		a2 := f[1*4+j] - f[3*4+j]
		a3 := f[1*4+j] + f[3*4+j]
		g[0*4+j] = a0 + a3
		g[1*4+j] = a1 + a2
		g[2*4+j] = a1 - a2
		g[3*4+j] = a0 - a3
	}
	m := qp % 6
	shift := qp / 6
	ls := levelScale4x4(m, 0, 0)
	var dc [16]int32
	for k := 0; k < 16; k++ {
		if shift >= 6 {
			dc[k] = (g[k] * ls) << uint(shift-6)
		} else {
			dc[k] = (g[k]*ls + (1 << uint(5-shift))) >> uint(6-shift)
		}
	}
	return dc
}

// inverseChromaDC — inverse 2x2 Hadamard over the chroma DC (4:2:0) and scaling
// qpC is the chroma QP. Order of c: [(0,0),(0,1),(1,0),(1,1)].
func inverseChromaDC(c [4]int32, qpC int) [4]int32 {
	f0 := c[0] + c[1] + c[2] + c[3]
	f1 := c[0] - c[1] + c[2] - c[3]
	f2 := c[0] + c[1] - c[2] - c[3]
	f3 := c[0] - c[1] - c[2] + c[3]
	m := qpC % 6
	shift := qpC / 6
	ls := levelScale4x4(m, 0, 0)
	scale := func(v int32) int32 { return ((v * ls) << uint(shift)) >> 5 }
	return [4]int32{scale(f0), scale(f1), scale(f2), scale(f3)}
}
