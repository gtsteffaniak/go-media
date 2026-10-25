package transform

// DequantInverse4x4 applies inverse quantize then 4×4 IDCT on coefficients in raster order.
func DequantInverse4x4(coeffs [16]int32, qp int, scalingList *[16]int32) [16]int32 {
	return InverseTransform4x4(Dequant4x4(coeffs, qp, scalingList))
}

// DequantInverse4x4FromScan is like DequantInverse4x4 but input levels are in H.264 scan order.
func DequantInverse4x4FromScan(scan [16]int32, qp int, scalingList *[16]int32) [16]int32 {
	var raster [16]int32
	for j := range 16 {
		raster[zigzag4x4[j]] = scan[j]
	}
	return DequantInverse4x4(raster, qp, scalingList)
}

// zigzag4x4 maps scan index to raster index (Table 8-13).
var zigzag4x4 = [16]int{
	0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15,
}
