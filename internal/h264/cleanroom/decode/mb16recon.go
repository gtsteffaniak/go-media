package decode

// decodeMB16 decodes and reconstructs a single I_16x16 macroblock:
// mb_pred → mb_qp_delta → residual (DC Hadamard + AC) → predict+residual into the plane.
func (sd *sliceDecoder) decodeMB16(addr int, mb MBType) error {
	r, f := sd.r, sd.frame
	mbX, mbY := addr%f.MbWidth, addr/f.MbWidth

	// mb_pred: intra_chroma_pred_mode (for 4:2:0/4:2:2).
	chromaPredMode := 0
	if f.ChromaArrayType == 1 || f.ChromaArrayType == 2 {
		chromaPredMode = int(r.UE())
	}
	// I_16x16: mb_qp_delta is always present.
	sd.qp = (sd.qp + int(r.SE()) + 52) % 52 // QpBdOffset=0 (8 bit)
	qp := sd.qp

	// ---- Luma ----
	top, left, corner, availL, availT := sd.lumaNeighbors16(mbX, mbY)
	pred := sd.predLuma[:]
	predictIntra16x16(pred, mb.Intra16x16PredMode, top, left, corner, availL, availT)

	// DC block (16 DC coefficients → inverse Hadamard).
	dcScan, _ := residualBlockCAVLC(r, sd.lumaNC(mbX, mbY, 0), 16)
	dcVals := inverseLumaDC(inverseScan4x4(dcScan), qp)

	for blk := 0; blk < 16; blk++ {
		bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
		gx, gy := mbX*4+bx, mbY*4+by
		var d [16]int32
		if mb.CBPLuma != 0 { // CBPLuma=15 → all AC are present
			acScan, tc := residualBlockCAVLC(r, sd.lumaNC(mbX, mbY, blk), 15)
			sd.nzLuma[gy*sd.w4+gx] = uint8(tc)
			d = dequant4x4(buildAC(acScan), qp)
		} else {
			sd.nzLuma[gy*sd.w4+gx] = 0
		}
		d[0] = dcVals[by*4+bx] // DC from Hadamard (already scaled)
		res := inverseTransform4x4(d)
		writeBlock(f.Y, f.StrideY, mbX*16+bx*4, mbY*16+by*4, pred, 16, bx*4, by*4, res)
	}

	// ---- Chroma ----
	switch f.ChromaArrayType {
	case 0: // monochrome — no chroma
	case 1: // 4:2:0
		sd.reconstructChroma420(mbX, mbY, chromaPredMode, mb.CBPChroma, qp)
	default:
		return ErrNotImplemented // 4:2:2/4:4:4 — later
	}
	return nil
}

// buildAC lays out 15 AC coefficients (scan positions 1..15) into a 4x4 raster
// (position 0 — DC — stays zero and is overwritten separately).
func buildAC(acScan []int32) [16]int32 {
	var scan16 [16]int32
	for k := 0; k < 15 && k < len(acScan); k++ {
		scan16[k+1] = acScan[k]
	}
	return inverseScan4x4(scan16[:])
}

// writeBlock adds prediction and residual (4x4) and writes into the plane with
// saturation. predStride is the width of the prediction buffer, (px0,py0) is the
// top-left of the block in the plane, (lx,ly) is the block offset inside the prediction buffer.
func writeBlock(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, res [16]int32) {
	for yy := 0; yy < 4; yy++ {
		for xx := 0; xx < 4; xx++ {
			v := int(pred[(ly+yy)*predStride+lx+xx]) + int(res[yy*4+xx])
			plane[(py0+yy)*stride+px0+xx] = clip1(v)
		}
	}
}

// reconstructChroma420 reconstructs both 4:2:0 chroma components (4 blocks of 4x4 each).
// Read order: first the DC of both components, then the AC of both.
func (sd *sliceDecoder) reconstructChroma420(mbX, mbY, predMode, cbpChroma, qpY int) {
	f, r := sd.frame, sd.r
	qpC := chromaQP(qpY, int(sd.pps.ChromaQPIndexOffset))
	const numBlk = 4 // 2x2 blocks of 4x4 per component
	comps := []struct {
		buf []byte
		nz  []uint8
	}{{f.Cb, sd.nzCb}, {f.Cr, sd.nzCr}}

	// DC of both components.
	dc := [2][numBlk]int32{}
	if cbpChroma&3 != 0 {
		for c := range comps {
			dcScan, _ := residualBlockCAVLC(r, -1, numBlk)
			var cc [4]int32
			copy(cc[:], dcScan[:numBlk])
			dc[c] = inverseChromaDC(cc, qpC)
		}
	}

	// AC + prediction + write per component.
	for c := range comps {
		top, left, corner, availL, availT := sd.chromaNeighbors(comps[c].buf, mbX, mbY)
		pred := sd.predChroma[:f.MbWidthC*f.MbHeightC]
		predictIntraChroma8x8(pred, predMode, top, left, corner, availL, availT)

		for blk := 0; blk < numBlk; blk++ {
			bx, by := blk%sd.cbpw, blk/sd.cbpw
			gx, gy := mbX*sd.cbpw+bx, mbY*(f.MbHeightC/4)+by
			var d [16]int32
			if cbpChroma == 2 {
				acScan, tc := residualBlockCAVLC(r, sd.chromaNC(comps[c].nz, mbX, mbY, blk), 15)
				comps[c].nz[gy*sd.cw4+gx] = uint8(tc)
				d = dequant4x4(buildAC(acScan), qpC)
			} else {
				comps[c].nz[gy*sd.cw4+gx] = 0
			}
			d[0] = dc[c][blk]
			res := inverseTransform4x4(d)
			writeBlock(comps[c].buf, f.StrideC, mbX*f.MbWidthC+bx*4, mbY*f.MbHeightC+by*4,
				pred, f.MbWidthC, bx*4, by*4, res)
		}
	}
}

// lumaNeighbors16 gathers neighboring reconstructed samples for predicting
// a 16x16 luma MB.
func (sd *sliceDecoder) lumaNeighbors16(mbX, mbY int) (top, left []byte, corner byte, availL, availT bool) {
	f := sd.frame
	top, left = sd.nTop16[:], sd.nLeft16[:]
	availL = mbX > 0 && sd.mbDecoded[mbY*f.MbWidth+mbX-1]
	availT = mbY > 0 && sd.mbDecoded[(mbY-1)*f.MbWidth+mbX]
	x0, y0 := mbX*16, mbY*16
	if availT {
		row := (y0 - 1) * f.StrideY
		for x := 0; x < 16; x++ {
			top[x] = f.Y[row+x0+x]
		}
	}
	if availL {
		for y := 0; y < 16; y++ {
			left[y] = f.Y[(y0+y)*f.StrideY+x0-1]
		}
	}
	if availL && availT {
		corner = f.Y[(y0-1)*f.StrideY+x0-1]
	}
	return
}

// chromaNeighbors — the same for a chroma plane (MbWidthC×MbHeightC).
func (sd *sliceDecoder) chromaNeighbors(buf []byte, mbX, mbY int) (top, left []byte, corner byte, availL, availT bool) {
	f := sd.frame
	w, hh := f.MbWidthC, f.MbHeightC
	top, left = sd.nTop16[:w], sd.nLeft16[:hh]
	availL = mbX > 0 && sd.mbDecoded[mbY*f.MbWidth+mbX-1]
	availT = mbY > 0 && sd.mbDecoded[(mbY-1)*f.MbWidth+mbX]
	x0, y0 := mbX*w, mbY*hh
	if availT {
		row := (y0 - 1) * f.StrideC
		for x := 0; x < w; x++ {
			top[x] = buf[row+x0+x]
		}
	}
	if availL {
		for y := 0; y < hh; y++ {
			left[y] = buf[(y0+y)*f.StrideC+x0-1]
		}
	}
	if availL && availT {
		corner = buf[(y0-1)*f.StrideC+x0-1]
	}
	return
}
