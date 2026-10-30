package mpeg4

// 8x8 inverse DCT matching ffmpeg's simple_idct (8-bit, int16 rows),
// written from the published algorithm: a row pass with 11-bit rounding
// shift and a DC-only shortcut, then a column pass with a 20-bit shift.
// The coefficients are round(cos(k*pi/16) * sqrt(2) * 2^14), with W4
// lowered by one as in the reference.
const (
	idctW1 = 22725
	idctW2 = 21407
	idctW3 = 19266
	idctW4 = 16383
	idctW5 = 12873
	idctW6 = 8867
	idctW7 = 4520

	idctRowShift = 11
	idctColShift = 20
)

func idctRow(row *[8]int16) {
	if row[1]|row[2]|row[3]|row[4]|row[5]|row[6]|row[7] == 0 {
		v := int16(int32(row[0]) << 3)
		for i := range row {
			row[i] = v
		}
		return
	}
	r0, r1, r2, r3 := int32(row[0]), int32(row[1]), int32(row[2]), int32(row[3])
	r4, r5, r6, r7 := int32(row[4]), int32(row[5]), int32(row[6]), int32(row[7])
	a0 := idctW4*r0 + (1 << (idctRowShift - 1))
	a1, a2, a3 := a0, a0, a0
	a0 += idctW2 * r2
	a1 += idctW6 * r2
	a2 -= idctW6 * r2
	a3 -= idctW2 * r2
	b0 := idctW1*r1 + idctW3*r3
	b1 := idctW3*r1 - idctW7*r3
	b2 := idctW5*r1 - idctW1*r3
	b3 := idctW7*r1 - idctW5*r3
	a0 += idctW4*r4 + idctW6*r6
	a1 += -idctW4*r4 - idctW2*r6
	a2 += -idctW4*r4 + idctW2*r6
	a3 += idctW4*r4 - idctW6*r6
	b0 += idctW5*r5 + idctW7*r7
	b1 += -idctW1*r5 - idctW5*r7
	b2 += idctW7*r5 + idctW3*r7
	b3 += idctW3*r5 - idctW1*r7
	row[0] = int16((a0 + b0) >> idctRowShift)
	row[7] = int16((a0 - b0) >> idctRowShift)
	row[1] = int16((a1 + b1) >> idctRowShift)
	row[6] = int16((a1 - b1) >> idctRowShift)
	row[2] = int16((a2 + b2) >> idctRowShift)
	row[5] = int16((a2 - b2) >> idctRowShift)
	row[3] = int16((a3 + b3) >> idctRowShift)
	row[4] = int16((a3 - b3) >> idctRowShift)
}

func clip8(v int32) byte {
	if uint32(v) > 255 {
		if v < 0 {
			return 0
		}
		return 255
	}
	return byte(v)
}

// idctPut transforms blk in place and writes clipped pixels to dst with
// the given stride.
func idctPut(blk *[64]int16, dst []byte, stride int) {
	for i := 0; i < 8; i++ {
		idctRow((*[8]int16)(blk[i*8 : i*8+8]))
	}
	for c := 0; c < 8; c++ {
		c0, c1, c2, c3 := int32(blk[c]), int32(blk[8+c]), int32(blk[16+c]), int32(blk[24+c])
		c4, c5, c6, c7 := int32(blk[32+c]), int32(blk[40+c]), int32(blk[48+c]), int32(blk[56+c])
		a0 := idctW4 * (c0 + ((1 << (idctColShift - 1)) / idctW4))
		a1, a2, a3 := a0, a0, a0
		a0 += idctW2*c2 + idctW4*c4 + idctW6*c6
		a1 += idctW6*c2 - idctW4*c4 - idctW2*c6
		a2 += -idctW6*c2 - idctW4*c4 + idctW2*c6
		a3 += -idctW2*c2 + idctW4*c4 - idctW6*c6
		b0 := idctW1*c1 + idctW3*c3 + idctW5*c5 + idctW7*c7
		b1 := idctW3*c1 - idctW7*c3 - idctW1*c5 - idctW5*c7
		b2 := idctW5*c1 - idctW1*c3 + idctW7*c5 + idctW3*c7
		b3 := idctW7*c1 - idctW5*c3 + idctW3*c5 - idctW1*c7
		d := dst[c:]
		d[0] = clip8((a0 + b0) >> idctColShift)
		d[stride] = clip8((a1 + b1) >> idctColShift)
		d[2*stride] = clip8((a2 + b2) >> idctColShift)
		d[3*stride] = clip8((a3 + b3) >> idctColShift)
		d[4*stride] = clip8((a3 - b3) >> idctColShift)
		d[5*stride] = clip8((a2 - b2) >> idctColShift)
		d[6*stride] = clip8((a1 - b1) >> idctColShift)
		d[7*stride] = clip8((a0 - b0) >> idctColShift)
	}
}
