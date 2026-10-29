package decode

import (
	"errors"

	h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"
)

// CABAC macroblock decoding for I-slices. Mirrors the CAVLC paths (decodeMB16 /
// decodeMBNxN / reconstructChroma420) but reads syntax via the CABAC engine; the
// reconstruction helpers (prediction, dequant, transform, deblocking) are reused.
// Context derivations are ported from OpenH264 (parse_mb_syn_cabac.cpp).

// runCabac iterates the macroblocks of a CABAC I-slice.
func (sd *sliceDecoder) runCabac() error {
	sd.r.AlignToByte() // cabac_alignment_one_bit
	sd.cb = &sd.cabacScratch
	sd.cb.resetCabacEngine(sd.r, int(sd.h.SliceQP), 0)
	mw := sd.frame.MbWidth
	addr := int(sd.h.FirstMB)
	total := mw * sd.frame.MbHeight
	for {
		if addr >= total {
			return errors.New("h264: macroblock address went out of frame bounds")
		}
		mbX, mbY := addr%mw, addr/mw
		leftNon4x4 := mbX > 0 && sd.mbDecoded[addr-1] && !sd.mbI4x4[addr-1]
		topNon4x4 := mbY > 0 && sd.mbDecoded[addr-mw] && !sd.mbI4x4[addr-mw]
		mb, err := decodeIMBType(uint32(sd.cb.parseIMBTypeCabac(leftNon4x4, topNon4x4)))
		if err != nil {
			return err
		}
		switch mb.Kind {
		case MbIPCM:
			return ErrNotImplemented // CABAC I_PCM is rare; not supported yet
		case MbI16x16:
			if err := sd.decodeMB16Cabac(addr, mb); err != nil {
				return err
			}
			sd.markMBRecon4(mbX, mbY)
			sd.mbQP[addr] = sd.qp
		case MbINxN:
			sd.mbI4x4[addr] = true
			if err := sd.decodeMBNxNCabac(addr); err != nil {
				return err
			}
			sd.mbQP[addr] = sd.qp
		}
		if err := sd.cb.err(); err != nil {
			return err
		}
		sd.mbDecoded[addr] = true
		sd.decodedMBs++
		if sd.cb.decodeTerminate() == 1 { // end_of_slice_flag
			break
		}
		addr++
	}
	return nil
}

func (sd *sliceDecoder) decodeMB16Cabac(addr int, mb MBType) error {
	f, cb := sd.frame, sd.cb
	mbX, mbY := addr%f.MbWidth, addr/f.MbWidth

	chromaPredMode := 0
	if f.ChromaArrayType == 1 || f.ChromaArrayType == 2 {
		chromaPredMode = cb.parseIntraChromaPredCabac(sd.chromaPredCtxInc(mbX, mbY))
	}
	sd.mbChromaPredMode[addr] = int8(chromaPredMode)

	qpDelta := cb.parseDeltaQpCabac(sd.lastQpDeltaNonZero) // always present for I_16x16
	sd.lastQpDeltaNonZero = qpDelta != 0
	sd.qp = (sd.qp + qpDelta + 52) % 52
	qp := sd.qp
	sd.mbCbp[addr] = mb.CBPChroma<<4 | mb.CBPLuma

	top, left, corner, availL, availT := sd.lumaNeighbors16(mbX, mbY)
	predictIntra16x16At(f.Y, f.StrideY, mbX*16, mbY*16, mb.Intra16x16PredMode, top, left, corner, availL, availT)

	cA, cB := sd.lumaDCCondTerm(mbX, mbY)
	dcScan, dcCount := residualBlockCABAC(sd, resI16LumaDC, cA, cB)
	sd.cbfLumaDC[addr] = dcCount > 0
	dcVals := inverseLumaDC(inverseScan4x4(dcScan), qp)

	var blocks [16][16]int32
	var nz [16]uint8
	for blk := 0; blk < 16; blk++ {
		bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
		gx, gy := mbX*4+bx, mbY*4+by
		if mb.CBPLuma != 0 {
			ca, cb2 := sd.lumaACCondTerm(mbX, mbY, blk)
			acScan, tc := residualBlockCABAC(sd, resI16LumaAC, ca, cb2)
			sd.nzLuma[gy*sd.w4+gx] = uint8(tc)
			nz[blk] = uint8(tc)
			blocks[blk] = dequant4x4(buildAC(acScan), qp)
		} else {
			sd.nzLuma[gy*sd.w4+gx] = 0
		}
		blocks[blk][0] = dcVals[by*4+bx]
	}
	h264k.IDCTAdd16LumaInPlace(f.Y, f.StrideY, mbX, mbY, &blocks, nz[:])

	switch f.ChromaArrayType {
	case 0:
	case 1:
		sd.reconstructChroma420Cabac(mbX, mbY, chromaPredMode, mb.CBPChroma, qp)
	default:
		return ErrNotImplemented
	}
	return nil
}

func (sd *sliceDecoder) decodeMBNxNCabac(addr int) error {
	f, cb := sd.frame, sd.cb
	mbX, mbY := addr%f.MbWidth, addr/f.MbWidth

	if sd.pps.Transform8x8Mode {
		sd.mbTransform8x8[addr] = sd.parseTransform8x8FlagCabac(mbX, mbY)
		if sd.mbTransform8x8[addr] {
			return sd.decodeMB8x8Cabac(addr)
		}
	}

	for blk := 0; blk < 16; blk++ {
		gx, gy := mbX*4+luma4x4BlockX[blk], mbY*4+luma4x4BlockY[blk]
		predMode := sd.predIntra4x4Mode(mbX, mbY, gx, gy)
		mode := predMode
		if v := cb.parseIntraPredLumaCabac(); v >= 0 {
			if v < predMode {
				mode = v
			} else {
				mode = v + 1
			}
		}
		sd.i4mode[gy*sd.w4+gx] = int8(mode)
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

	for blk := 0; blk < 16; blk++ {
		bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
		gx, gy := mbX*4+bx, mbY*4+by
		top, left, corner, availTop, availLeft := sd.luma4x4Neighbors(gx, gy)
		var pred [16]byte
		predictIntra4x4(pred[:], int(sd.i4mode[gy*sd.w4+gx]), top, left, corner, availLeft, availTop)

		var d [16]int32
		var tc uint8
		if cbpLuma&(1<<uint(blk/4)) != 0 {
			ca, cbB := sd.lumaACCondTerm(mbX, mbY, blk)
			scan, n := residualBlockCABAC(sd, resLuma4x4, ca, cbB)
			tc = uint8(n)
			sd.nzLuma[gy*sd.w4+gx] = tc
			d = dequant4x4(inverseScan4x4(scan), qp)
		} else {
			sd.nzLuma[gy*sd.w4+gx] = 0
		}
		px0, py0 := gx*4, gy*4
		h264k.IDCTAdd4x4Luma(f.Y, f.StrideY, px0, py0, pred[:], 4, 0, 0, d, tc)
		sd.recon4[gy*sd.w4+gx] = true
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

func (sd *sliceDecoder) reconstructChroma420Cabac(mbX, mbY, predMode, cbpChroma, qpY int) {
	f := sd.frame
	qpC := chromaQP(qpY, int(sd.pps.ChromaQPIndexOffset))
	const numBlk = 4
	comps := []struct {
		buf   []byte
		nz    []uint8
		cbf   []bool
		resDC int
		resAC int
	}{
		{f.Cb, sd.nzCb, sd.cbfCbDC, resChromaDCU, resChromaACU},
		{f.Cr, sd.nzCr, sd.cbfCrDC, resChromaDCV, resChromaACV},
	}

	dc := [2][numBlk]int32{}
	if cbpChroma&3 != 0 {
		for c := range comps {
			cA, cB := sd.chromaDCCondTerm(comps[c].cbf, mbX, mbY)
			dcScan, cnt := residualBlockCABAC(sd, comps[c].resDC, cA, cB)
			comps[c].cbf[mbY*f.MbWidth+mbX] = cnt > 0
			var cc [4]int32
			copy(cc[:], dcScan[:numBlk])
			dc[c] = inverseChromaDC(cc, qpC)
		}
	}

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
				ca, cbB := sd.chromaACCondTerm(comps[c].nz, mbX, mbY, blk)
				acScan, n := residualBlockCABAC(sd, comps[c].resAC, ca, cbB)
				tc := uint8(n)
				comps[c].nz[gy*sd.cw4+gx] = tc
				nz[blk] = tc
				blocks[blk] = dequant4x4(buildAC(acScan), qpC)
			} else {
				comps[c].nz[gy*sd.cw4+gx] = 0
			}
			blocks[blk][0] = dc[c][blk]
		}
		h264k.IDCTAdd4Chroma420InPlace(comps[c].buf, f.StrideC, cx0, cy0, &blocks, nz[:])
	}
}

// parseCbpCabac decodes coded_block_pattern via CABAC, returning (cbpLuma, cbpChroma).
func (sd *sliceDecoder) parseCbpCabac(mbX, mbY int) (int, int) {
	cb, mw := sd.cb, sd.frame.MbWidth
	addr := mbY*mw + mbX
	leftAvail := mbX > 0 && sd.mbDecoded[addr-1]
	topAvail := mbY > 0 && sd.mbDecoded[addr-mw]
	var leftCbp, topCbp int
	var leftPCM, topPCM bool
	if leftAvail {
		leftCbp, leftPCM = sd.mbCbp[addr-1], sd.mbIsPCM[addr-1]
	}
	if topAvail {
		topCbp, topPCM = sd.mbCbp[addr-mw], sd.mbIsPCM[addr-mw]
	}

	bTop0 := topAvail && !topPCM && topCbp&(1<<2) == 0
	bTop1 := topAvail && !topPCM && topCbp&(1<<3) == 0
	aLeft0 := leftAvail && !leftPCM && leftCbp&(1<<1) == 0
	aLeft1 := leftAvail && !leftPCM && leftCbp&(1<<3) == 0

	cbp := 0
	bit0 := int(cb.decodeDecision(ctxCbp + b2i(aLeft0) + 2*b2i(bTop0)))
	cbp += bit0
	bit1 := int(cb.decodeDecision(ctxCbp + b2i(bit0 == 0) + 2*b2i(bTop1)))
	cbp += bit1 << 1
	bit2 := int(cb.decodeDecision(ctxCbp + b2i(aLeft1) + 2*b2i(bit0 == 0)))
	cbp += bit2 << 2
	bit3 := int(cb.decodeDecision(ctxCbp + b2i(bit2 == 0) + 2*b2i(bit1 == 0)))
	cbp += bit3 << 3

	if sd.frame.ChromaArrayType == 0 {
		return cbp & 0xF, cbp >> 4
	}

	iB := b2i(topAvail && (topPCM || topCbp>>4 != 0))
	iA := b2i(leftAvail && (leftPCM || leftCbp>>4 != 0))
	if cb.decodeDecision(ctxCbp+ctxNumCbp+iA+2*iB) == 1 {
		iB = b2i(topAvail && (topPCM || topCbp>>4 == 2))
		iA = b2i(leftAvail && (leftPCM || leftCbp>>4 == 2))
		bit5 := int(cb.decodeDecision(ctxCbp + 2*ctxNumCbp + iA + 2*iB))
		cbp += 1 << (4 + bit5)
	}
	return cbp & 0xF, cbp >> 4
}

// chromaPredCtxInc — context increment for intra_chroma_pred_mode bin 0.
func (sd *sliceDecoder) chromaPredCtxInc(mbX, mbY int) int {
	mw := sd.frame.MbWidth
	addr := mbY*mw + mbX
	inc := 0
	if mbX > 0 && sd.mbDecoded[addr-1] && !sd.mbIsPCM[addr-1] &&
		sd.mbChromaPredMode[addr-1] >= 1 && sd.mbChromaPredMode[addr-1] <= 3 {
		inc++
	}
	if mbY > 0 && sd.mbDecoded[addr-mw] && !sd.mbIsPCM[addr-mw] &&
		sd.mbChromaPredMode[addr-mw] >= 1 && sd.mbChromaPredMode[addr-mw] <= 3 {
		inc++
	}
	return inc
}

// lumaDCCondTerm — coded_block_flag context increments for the I_16x16 luma DC block.
func (sd *sliceDecoder) lumaDCCondTerm(mbX, mbY int) (int, int) {
	mw := sd.frame.MbWidth
	addr := mbY*mw + mbX
	condA, condB := 1, 1 // intra default when neighbor unavailable
	if mbX > 0 && sd.mbDecoded[addr-1] {
		condA = b2i(sd.cbfLumaDC[addr-1])
	}
	if mbY > 0 && sd.mbDecoded[addr-mw] {
		condB = b2i(sd.cbfLumaDC[addr-mw])
	}
	return condA, condB
}

// lumaACCondTerm — coded_block_flag context increments for a luma 4x4/AC block.
func (sd *sliceDecoder) lumaACCondTerm(mbX, mbY, blk int) (int, int) {
	bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
	gx, gy := mbX*4+bx, mbY*4+by
	mw := sd.frame.MbWidth
	condA, condB := 1, 1
	if bx > 0 || (mbX > 0 && sd.mbDecoded[mbY*mw+mbX-1]) {
		condA = b2i(sd.nzLuma[gy*sd.w4+gx-1] > 0)
	}
	if by > 0 || (mbY > 0 && sd.mbDecoded[(mbY-1)*mw+mbX]) {
		condB = b2i(sd.nzLuma[(gy-1)*sd.w4+gx] > 0)
	}
	return condA, condB
}

// chromaACCondTerm — coded_block_flag context increments for a chroma AC block.
func (sd *sliceDecoder) chromaACCondTerm(nz []uint8, mbX, mbY, blk int) (int, int) {
	bx, by := blk%sd.cbpw, blk/sd.cbpw
	gx, gy := mbX*sd.cbpw+bx, mbY*(sd.frame.MbHeightC/4)+by
	mw := sd.frame.MbWidth
	condA, condB := 1, 1
	if bx > 0 || (mbX > 0 && sd.mbDecoded[mbY*mw+mbX-1]) {
		condA = b2i(nz[gy*sd.cw4+gx-1] > 0)
	}
	if by > 0 || (mbY > 0 && sd.mbDecoded[(mbY-1)*mw+mbX]) {
		condB = b2i(nz[(gy-1)*sd.cw4+gx] > 0)
	}
	return condA, condB
}

// chromaDCCondTerm — coded_block_flag context increments for a chroma DC block.
func (sd *sliceDecoder) chromaDCCondTerm(cbf []bool, mbX, mbY int) (int, int) {
	mw := sd.frame.MbWidth
	addr := mbY*mw + mbX
	condA, condB := 1, 1
	if mbX > 0 && sd.mbDecoded[addr-1] {
		condA = b2i(cbf[addr-1])
	}
	if mbY > 0 && sd.mbDecoded[addr-mw] {
		condB = b2i(cbf[addr-mw])
	}
	return condA, condB
}
