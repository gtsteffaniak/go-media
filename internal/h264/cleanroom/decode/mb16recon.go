package decode

import h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"

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
	px0, py0 := mbX*16, mbY*16
	predictIntra16x16At(f.Y, f.StrideY, px0, py0, mb.Intra16x16PredMode, top, left, corner, availL, availT)

	// DC block (16 DC coefficients → inverse Hadamard).
	_ = sd.residualBlockCAVLC(r, sd.lumaNC(mbX, mbY, 0), 16)
	dcVals := inverseLumaDC(inverseScan4x4(sd.coeffScratch[:16]), qp)

	var blocks [16][16]int32
	var nz [16]uint8
	for blk := 0; blk < 16; blk++ {
		bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
		gx, gy := mbX*4+bx, mbY*4+by
		if mb.CBPLuma != 0 { // CBPLuma=15 → all AC are present
			tc := sd.residualBlockCAVLC(r, sd.lumaNC(mbX, mbY, blk), 15)
			sd.nzLuma[gy*sd.w4+gx] = uint8(tc)
			nz[blk] = uint8(tc)
			blocks[blk] = dequant4x4(buildAC(sd.coeffScratch[:15]), qp)
		} else {
			sd.nzLuma[gy*sd.w4+gx] = 0
			nz[blk] = 0
		}
		blocks[blk][0] = dcVals[by*4+bx] // DC from Hadamard (already scaled)
	}
	h264k.IDCTAdd16LumaInPlace(f.Y, f.StrideY, mbX, mbY, &blocks, nz[:])

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
	h264k.WriteBlock4x4(plane, stride, px0, py0, pred, predStride, lx, ly, res)
}

// reconstructChroma420 reconstructs both 4:2:0 chroma components (4 blocks of 4x4 each).
// Read order: first the DC of both components, then the AC of both.
func (sd *sliceDecoder) reconstructChroma420(mbX, mbY, predMode, cbpChroma, qpY int) {
	f, r := sd.frame, sd.r
	// Index 0 is Cb, index 1 is Cr. Each has its own offset.
	qpC := [2]int{chromaQPFor(sd.pps, 0, qpY), chromaQPFor(sd.pps, 1, qpY)}
	const numBlk = 4 // 2x2 blocks of 4x4 per component
	comps := []struct {
		buf []byte
		nz  []uint8
	}{{f.Cb, sd.nzCb}, {f.Cr, sd.nzCr}}

	// DC of both components.
	dc := [2][numBlk]int32{}
	if cbpChroma&3 != 0 {
		for c := range comps {
			_ = sd.residualBlockCAVLC(r, -1, numBlk)
			var cc [4]int32
			copy(cc[:], sd.coeffScratch[:numBlk])
			dc[c] = inverseChromaDC(cc, qpC[c])
		}
	}

	// AC + prediction + write per component.
	for c := range comps {
		top, left, corner, availL, availT := sd.chromaNeighbors(comps[c].buf, mbX, mbY)
		cx0, cy0 := mbX*f.MbWidthC, mbY*f.MbHeightC
		predictIntraChroma8x8At(comps[c].buf, f.StrideC, cx0, cy0, predMode, top, left, corner, availL, availT)

		var blocks [4][16]int32
		var nz [4]uint8
		for blk := 0; blk < numBlk; blk++ {
			bx, by := blk%sd.cbpw, blk/sd.cbpw
			gx, gy := mbX*sd.cbpw+bx, mbY*(f.MbHeightC/4)+by
			if cbpChroma == 2 {
				tc := sd.residualBlockCAVLC(r, sd.chromaNC(comps[c].nz, mbX, mbY, blk), 15)
				comps[c].nz[gy*sd.cw4+gx] = uint8(tc)
				nz[blk] = uint8(tc)
				blocks[blk] = dequant4x4(buildAC(sd.coeffScratch[:15]), qpC[c])
			} else {
				comps[c].nz[gy*sd.cw4+gx] = 0
			}
			blocks[blk][0] = dc[c][blk]
		}
		h264k.IDCTAdd4Chroma420InPlace(comps[c].buf, f.StrideC, cx0, cy0, &blocks, nz[:])
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
