package h264

// IDCTDCAdd4x4 adds a DC-only residual (single nonzero coeff at index 0) to prediction.
func IDCTDCAdd4x4(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, dcCoeff int32) {
	if dcCoeff == 0 {
		for yy := 0; yy < 4; yy++ {
			row := (py0 + yy) * stride
			poff := (ly + yy) * predStride
			for xx := 0; xx < 4; xx++ {
				plane[row+px0+xx] = pred[poff+lx+xx]
			}
		}
		return
	}
	var d [16]int32
	d[0] = dcCoeff
	res := inverseTransform4x4Residual(d)
	WriteBlock4x4(plane, stride, px0, py0, pred, predStride, lx, ly, res)
}

// IDCTAdd4x4Luma reconstructs one 4×4 luma block; tc is TotalCoeff for AC (0 = no AC parsed).
func IDCTAdd4x4Luma(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, d [16]int32, tc uint8) {
	if tc == 0 && d[0] == 0 {
		IDCTDCAdd4x4(plane, stride, px0, py0, pred, predStride, lx, ly, 0)
		return
	}
	if tc == 0 {
		IDCTDCAdd4x4(plane, stride, px0, py0, pred, predStride, lx, ly, d[0])
		return
	}
	IDCTAdd4x4Block(plane, stride, px0, py0, pred, predStride, lx, ly, d)
}

// IDCTAdd4Chroma420 reconstructs four 4×4 chroma blocks in one 8×8 MB region (4:2:0).
func IDCTAdd4Chroma420(plane []byte, stride, px0, py0 int, pred []byte, predStride int, blocks *[4][16]int32, nz []uint8) {
	for blk := 0; blk < 4; blk++ {
		bx, by := blk%2, blk/2
		lx, ly := bx*4, by*4
		IDCTAdd4x4Luma(plane, stride, px0+bx*4, py0+by*4, pred, predStride, lx, ly, blocks[blk], nz[blk])
	}
}

// IDCTAdd4Chroma420InPlace adds residuals; plane already holds intra prediction.
func IDCTAdd4Chroma420InPlace(plane []byte, stride, px0, py0 int, blocks *[4][16]int32, nz []uint8) {
	if simdIDCTAddPair && KernelsUseSIMD {
		for by := 0; by < 2; by++ {
			i := by * 2
			if nz[i] == 0 && blocks[i][0] == 0 && nz[i+1] == 0 && blocks[i+1][0] == 0 {
				continue
			}
			idctAdd4x4PairSIMD(plane, (py0+by*4)*stride+px0, stride, &blocks[i], &blocks[i+1])
		}
		return
	}
	for blk := 0; blk < 4; blk++ {
		bx, by := blk%2, blk/2
		d := blocks[blk]
		if nz[blk] == 0 && d[0] == 0 {
			continue
		}
		res := inverseTransform4x4Residual(d)
		addResidual4x4(plane, stride, px0+bx*4, py0+by*4, res)
	}
}

// IDCTAdd4x4Block runs full 4×4 IDCT on dequantized coeffs d and adds to pred at (lx,ly).
func IDCTAdd4x4Block(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, d [16]int32) {
	res := inverseTransform4x4Residual(d)
	WriteBlock4x4(plane, stride, px0, py0, pred, predStride, lx, ly, res)
}

func addResidual4x4(plane []byte, stride, px0, py0 int, res [16]int32) {
	for yy := 0; yy < 4; yy++ {
		row := (py0+yy)*stride + px0
		for xx := 0; xx < 4; xx++ {
			v := int(plane[row+xx]) + int(res[yy*4+xx])
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			plane[row+xx] = byte(v)
		}
	}
}

// IDCTAdd16LumaInPlace adds IDCT residuals to plane that already holds intra prediction.
// blocks and nz are indexed in raster order (by*4+bx), not luma4x4BlkIdx order.
func IDCTAdd16LumaInPlace(plane []byte, stride, mbX, mbY int, blocks *[16][16]int32, nz []uint8) {
	if simdIDCTAddQuad && KernelsUseAVX512 {
		for by := 0; by < 4; by++ {
			i := by * 4
			if nz[i]|nz[i+1]|nz[i+2]|nz[i+3] == 0 && blocks[i][0]|blocks[i+1][0]|blocks[i+2][0]|blocks[i+3][0] == 0 {
				continue
			}
			idctAdd4x4QuadAVX512(plane, (mbY*16+by*4)*stride+mbX*16, stride, (*[4][16]int32)(blocks[i:i+4]))
		}
		return
	}
	if simdIDCTAddPair && KernelsUseSIMD {
		for i := 0; i < 16; i += 2 {
			if nz[i] == 0 && blocks[i][0] == 0 && nz[i+1] == 0 && blocks[i+1][0] == 0 {
				continue
			}
			bx, by := i%4, i/4
			idctAdd4x4PairSIMD(plane, (mbY*16+by*4)*stride+mbX*16+bx*4, stride, &blocks[i], &blocks[i+1])
		}
		return
	}
	for blk := 0; blk < 16; blk++ {
		bx, by := blk%4, blk/4
		px0, py0 := mbX*16+bx*4, mbY*16+by*4
		d := blocks[blk]
		if nz[blk] == 0 && d[0] == 0 {
			continue
		}
		res := inverseTransform4x4Residual(d)
		addResidual4x4(plane, stride, px0, py0, res)
	}
}

// IDCTAdd16Luma reconstructs I_16x16 luma: pred is 16×16 in predStride; blocks[blk] are
// dequantized 4×4 coeffs (DC already in [0]); nz[blk] is TotalCoeff for AC (0 = no AC parsed).
// Skips a block only when nz==0 and blocks[blk][0]==0.
func IDCTAdd16Luma(plane []byte, stride, mbX, mbY int, pred []byte, predStride int, blocks *[16][16]int32, nz []uint8) {
	for blk := 0; blk < 16; blk++ {
		bx, by := blk%4, blk/4
		px0, py0 := mbX*16+bx*4, mbY*16+by*4
		lx, ly := bx*4, by*4
		d := blocks[blk]
		tc := nz[blk]
		if tc == 0 && d[0] == 0 {
			IDCTDCAdd4x4(plane, stride, px0, py0, pred, predStride, lx, ly, 0)
			continue
		}
		if tc == 0 {
			IDCTDCAdd4x4(plane, stride, px0, py0, pred, predStride, lx, ly, d[0])
			continue
		}
		IDCTAdd4x4Block(plane, stride, px0, py0, pred, predStride, lx, ly, d)
	}
}

// IDCT8x8Add adds an 8×8 residual (post-IDCT) to pred and writes into plane at (px0,py0).
func IDCT8x8Add(plane []byte, stride, px0, py0 int, pred []byte, res [64]int32) {
	for yy := 0; yy < 8; yy++ {
		row := (py0+yy)*stride + px0
		poff := yy * 8
		for xx := 0; xx < 8; xx++ {
			v := int(pred[poff+xx]) + int(res[poff+xx])
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			plane[row+xx] = byte(v)
		}
	}
}

// idctAdd4x4PairPortable is the reference for idctAdd4x4PairSIMD.
func idctAdd4x4PairPortable(plane []byte, off, stride int, a, b *[16]int32) {
	ra := inverseTransform4x4ResidualPortable(*a)
	rb := inverseTransform4x4ResidualPortable(*b)
	addResidual4x4(plane, stride, off, 0, ra)
	addResidual4x4(plane, stride, off+4, 0, rb)
}

// idctAdd4x4QuadPortable is the reference for idctAdd4x4QuadAVX512.
func idctAdd4x4QuadPortable(plane []byte, off, stride int, q *[4][16]int32) {
	idctAdd4x4PairPortable(plane, off, stride, &q[0], &q[1])
	idctAdd4x4PairPortable(plane, off+8, stride, &q[2], &q[3])
}
