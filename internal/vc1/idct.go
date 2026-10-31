package vc1

// invTrans8x8 is the VC-1 8x8 inverse transform (SMPTE 421M 8.1.4). The
// coefficient block is stored transposed (see the transposed scan tables), so
// the first pass walks memory columns and the second writes output rows.
// Arithmetic is done in int to avoid int16 overflow on the 12x/16x products.
func invTrans8x8(block *[64]int16) {
	var temp [64]int
	for i := 0; i < 8; i++ {
		s0, s1, s2, s3 := int(block[i]), int(block[i+8]), int(block[i+16]), int(block[i+24])
		s4, s5, s6, s7 := int(block[i+32]), int(block[i+40]), int(block[i+48]), int(block[i+56])
		t1 := 12*(s0+s4) + 4
		t2 := 12*(s0-s4) + 4
		t3 := 16*s2 + 6*s6
		t4 := 6*s2 - 16*s6
		t5, t6, t7, t8 := t1+t3, t2+t4, t2-t4, t1-t3
		t1 = 16*s1 + 15*s3 + 9*s5 + 4*s7
		t2 = 15*s1 - 4*s3 - 16*s5 - 9*s7
		t3 = 9*s1 - 16*s3 + 4*s5 + 15*s7
		t4 = 4*s1 - 9*s3 + 15*s5 - 16*s7
		dst := temp[i*8 : i*8+8]
		dst[0] = (t5 + t1) >> 3
		dst[1] = (t6 + t2) >> 3
		dst[2] = (t7 + t3) >> 3
		dst[3] = (t8 + t4) >> 3
		dst[4] = (t8 - t4) >> 3
		dst[5] = (t7 - t3) >> 3
		dst[6] = (t6 - t2) >> 3
		dst[7] = (t5 - t1) >> 3
	}
	for i := 0; i < 8; i++ {
		s0, s1, s2, s3 := temp[i], temp[i+8], temp[i+16], temp[i+24]
		s4, s5, s6, s7 := temp[i+32], temp[i+40], temp[i+48], temp[i+56]
		t1 := 12*(s0+s4) + 64
		t2 := 12*(s0-s4) + 64
		t3 := 16*s2 + 6*s6
		t4 := 6*s2 - 16*s6
		t5, t6, t7, t8 := t1+t3, t2+t4, t2-t4, t1-t3
		t1 = 16*s1 + 15*s3 + 9*s5 + 4*s7
		t2 = 15*s1 - 4*s3 - 16*s5 - 9*s7
		t3 = 9*s1 - 16*s3 + 4*s5 + 15*s7
		t4 = 4*s1 - 9*s3 + 15*s5 - 16*s7
		block[i] = int16((t5 + t1) >> 7)
		block[i+8] = int16((t6 + t2) >> 7)
		block[i+16] = int16((t7 + t3) >> 7)
		block[i+24] = int16((t8 + t4) >> 7)
		block[i+32] = int16((t8 - t4 + 1) >> 7)
		block[i+40] = int16((t7 - t3 + 1) >> 7)
		block[i+48] = int16((t6 - t2 + 1) >> 7)
		block[i+56] = int16((t5 - t1 + 1) >> 7)
	}
}

func clipU8(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}
