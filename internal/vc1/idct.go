package vc1

func invTrans8x8(block *[64]int16) {
	var temp [64]int16
	for i := 0; i < 8; i++ {
		src := block[i:]
		dst := temp[i*8:]
		t1 := 12*(src[0]+src[32]) + 4
		t2 := 12*(src[0]-src[32]) + 4
		t3 := 16*src[16] + 6*src[48]
		t4 := 6*src[16] - 16*src[48]
		t5, t6, t7, t8 := t1+t3, t2+t4, t2-t4, t1-t3
		t1 = 16*src[8] + 15*src[24] + 9*src[40] + 4*src[56]
		t2 = 15*src[8] - 4*src[24] - 16*src[40] - 9*src[56]
		t3 = 9*src[8] - 16*src[24] + 4*src[40] + 15*src[56]
		t4 = 4*src[8] - 9*src[24] + 15*src[40] - 16*src[56]
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
		src := temp[i:]
		dst := block[i:]
		t1 := 12*(src[0]+src[32]) + 64
		t2 := 12*(src[0]-src[32]) + 64
		t3 := 16*src[16] + 6*src[48]
		t4 := 6*src[16] - 16*src[48]
		t5, t6, t7, t8 := t1+t3, t2+t4, t2-t4, t1-t3
		t1 = 16*src[8] + 15*src[24] + 9*src[40] + 4*src[56]
		t2 = 15*src[8] - 4*src[24] - 16*src[40] - 9*src[56]
		t3 = 9*src[8] - 16*src[24] + 4*src[40] + 15*src[56]
		t4 = 4*src[8] - 9*src[24] + 15*src[40] - 16*src[56]
		dst[0] = (t5 + t1) >> 7
		dst[1] = (t6 + t2) >> 7
		dst[2] = (t7 + t3) >> 7
		dst[3] = (t8 + t4) >> 7
		dst[4] = (t8 - t4) >> 7
		dst[5] = (t7 - t3) >> 7
		dst[6] = (t6 - t2) >> 7
		dst[7] = (t5 - t1) >> 7
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
