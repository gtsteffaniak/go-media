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
	ls := &levelScaleTab4x4[m]
	for idx, c := range coef {
		if c != 0 {
			d[idx] = dequantCoeffAt(c, ls[idx], shift)
		}
	}
	return d
}

var levelScaleTab4x4 = func() (t [6][16]int32) {
	for m := range t {
		for idx := range t[m] {
			t[m][idx] = levelScale4x4(m, idx/4, idx%4)
		}
	}
	return
}()
var zigzag4x4K = [16]uint8{0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15}

// DequantScan4x4 maps scan-order coefficients to raster order and dequantizes
// them in one pass, touching only nonzero coefficients.
func DequantScan4x4(scan []int32, qp int) [16]int32 {
	var d [16]int32
	m, shift := qp%6, qp/6
	scan = scan[:16]
	for s, c := range scan {
		if c != 0 {
			idx := zigzag4x4K[s]
			d[idx] = dequantCoeffAt(c, levelScaleTab4x4[m][idx], shift)
		}
	}
	return d
}
