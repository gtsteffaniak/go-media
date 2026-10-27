package h264

var normAdjust4x4 = [6][3]int32{
	{10, 16, 13},
	{11, 18, 14},
	{13, 20, 16},
	{14, 23, 18},
	{16, 25, 20},
	{18, 29, 23},
}

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

func levelScale4x4(m, i, j int) int32 {
	return 16 * normAdjust4x4[m][posClass4x4(i, j)]
}

// Dequant4x4 scales H.264 4×4 coefficients (flat matrix).
func Dequant4x4(coef [16]int32, qp int) [16]int32 {
	return dequant4x4(coef, qp)
}

func dequant4x4(coef [16]int32, qp int) [16]int32 {
	if KernelSIMD("dequant4x4") {
		return dequant4x4SIMD(coef, qp)
	}
	return dequant4x4Portable(coef, qp)
}

// dequantCoeffAt uses int32 fast path for typical coeffs; int64 only when product may overflow.
func dequantCoeffAt(c, ls int32, shift int) int32 {
	const safeC = int32(1 << 18) // conservative bound for ls≤464 and shift≤6
	if c >= -safeC && c <= safeC {
		var out int32
		if shift >= 4 {
			out = (c * ls) << uint(shift-4)
		} else {
			out = (c*ls + (1 << uint(3-shift))) >> uint(4-shift)
		}
		return saturateDequantCoeff(out)
	}
	c64 := int64(c)
	ls64 := int64(ls)
	var out int64
	if shift >= 4 {
		out = (c64 * ls64) << uint(shift-4)
	} else {
		out = (c64*ls64 + (1 << uint(3-shift))) >> uint(4-shift)
	}
	return saturateDequantCoeff(int32(out))
}

func saturateDequantCoeff(v int32) int32 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return v
}

func dequant4x4Portable(coef [16]int32, qp int) [16]int32 {
	m := qp % 6
	shift := qp / 6
	var d [16]int32
	for idx := 0; idx < 16; idx++ {
		ls := levelScale4x4(m, idx/4, idx%4)
		c := coef[idx]
		d[idx] = dequantCoeffAt(c, ls, shift)
	}
	return d
}
