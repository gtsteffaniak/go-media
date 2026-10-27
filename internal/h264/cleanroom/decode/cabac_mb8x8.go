package decode

import h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"

// High-profile Intra_8x8 macroblock decoding (CABAC). One transform_size_8x8
// luma block per 8x8 quadrant: 8x8 intra prediction + 8x8 residual/transform.
// Chroma stays on the 4x4 path.

// parseTransform8x8FlagCabac decodes transform_size_8x8_flag.
func (sd *sliceDecoder) parseTransform8x8FlagCabac(mbX, mbY int) bool {
	mw := sd.frame.MbWidth
	addr := mbY*mw + mbX
	inc := 0
	if mbX > 0 && sd.mbDecoded[addr-1] && sd.mbTransform8x8[addr-1] {
		inc++
	}
	if mbY > 0 && sd.mbDecoded[addr-mw] && sd.mbTransform8x8[addr-mw] {
		inc++
	}
	return sd.cb.decodeDecision(ctxTransform8x8+inc) == 1
}

func (sd *sliceDecoder) decodeMB8x8Cabac(addr int) error {
	f, cb := sd.frame, sd.cb
	mbX, mbY := addr%f.MbWidth, addr/f.MbWidth

	// mb_pred: 4 Intra8x8PredMode (one per 8x8 block); store each on all 4 sub-4x4.
	for blk8 := 0; blk8 < 4; blk8++ {
		gx, gy := mbX*4+(blk8%2)*2, mbY*4+(blk8/2)*2
		predMode := sd.predIntra4x4Mode(mbX, mbY, gx, gy)
		mode := predMode
		if v := cb.parseIntraPredLumaCabac(); v >= 0 {
			if v < predMode {
				mode = v
			} else {
				mode = v + 1
			}
		}
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				sd.i4mode[(gy+dy)*sd.w4+gx+dx] = int8(mode)
			}
		}
	}

	chromaPredMode := 0
	if f.ChromaArrayType == 1 || f.ChromaArrayType == 2 {
		chromaPredMode = cb.parseIntraChromaPredCabac(sd.chromaPredCtxInc(mbX, mbY))
	}
	sd.mbChromaPredMode[addr] = int8(chromaPredMode)

	cbpLuma, cbpChroma := sd.parseCbpCabac(mbX, mbY)
	sd.mbCbp[addr] = cbpChroma<<4 | cbpLuma
	if cbpLuma != 0 || cbpChroma != 0 {
		qpDelta := cb.parseDeltaQpCabac(sd.lastQpDeltaNonZero)
		sd.lastQpDeltaNonZero = qpDelta != 0
		sd.qp = (sd.qp + qpDelta + 52) % 52
	} else {
		sd.lastQpDeltaNonZero = false
	}
	qp := sd.qp

	for blk8 := 0; blk8 < 4; blk8++ {
		b8x, b8y := blk8%2, blk8/2
		gx, gy := mbX*4+b8x*2, mbY*4+b8y*2
		px0, py0 := mbX*16+b8x*8, mbY*16+b8y*8

		top, left, corner, availTop, availLeft, availTL, availTR := sd.luma8x8Neighbors(gx, gy, px0, py0)
		ft, fl, fc := filterRef8x8(top, left, corner, availTop, availLeft, availTL)
		_ = availTR
		var pred [64]byte
		predictIntra8x8(pred[:], int(sd.i4mode[gy*sd.w4+gx]), ft, fl, fc, availLeft, availTop)

		var res [64]int32
		var tc int
		if cbpLuma&(1<<uint(blk8)) != 0 {
			scan, n := residualBlockCABAC(sd, resLumaDC8, 0, 0)
			tc = n
			res = inverseTransform8x8(dequant8x8(inverseScan8x8(scan), qp))
		}
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				sd.nzLuma[(gy+dy)*sd.w4+gx+dx] = uint8(tc)
				sd.recon4[(gy+dy)*sd.w4+gx+dx] = true
			}
		}
		h264k.IDCT8x8Add(f.Y, f.StrideY, px0, py0, pred[:], res)
	}

	switch f.ChromaArrayType {
	case 0:
	case 1:
		sd.reconstructChroma420Cabac(mbX, mbY, chromaPredMode, cbpChroma, qp)
	default:
		return ErrNotImplemented
	}
	return nil
}

// luma8x8Neighbors gathers references for an 8x8 luma block: top (16 samples
// with the top-right or p[7,-1] substitution), left (8), corner.
func (sd *sliceDecoder) luma8x8Neighbors(gx, gy, px0, py0 int) (top, left []byte, corner byte, availTop, availLeft, availTL, availTR bool) {
	f := sd.frame
	top = sd.nTop8[:]
	left = sd.nLeft8[:]
	availTop = gy > 0 && sd.recon4[(gy-1)*sd.w4+gx]
	availLeft = gx > 0 && sd.recon4[gy*sd.w4+gx-1]
	availTL = gx > 0 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx-1]
	availTR = gx+2 < sd.w4 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx+2]

	if availTop {
		row := (py0 - 1) * f.StrideY
		for i := 0; i < 8; i++ {
			top[i] = f.Y[row+px0+i]
		}
	}
	if availTR {
		row := (py0 - 1) * f.StrideY
		for i := 0; i < 8; i++ {
			top[8+i] = f.Y[row+px0+8+i]
		}
	} else if availTop {
		for i := 0; i < 8; i++ {
			top[8+i] = top[7]
		}
	}
	if availLeft {
		for i := 0; i < 8; i++ {
			left[i] = f.Y[(py0+i)*f.StrideY+px0-1]
		}
	}
	if availTL {
		corner = f.Y[(py0-1)*f.StrideY+px0-1]
	}
	return
}
