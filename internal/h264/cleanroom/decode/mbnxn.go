package decode

import (
	"errors"

	h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"
)

// decodeMBNxN decodes and reconstructs an I_NxN (intra 4x4) macroblock:
// 16 prediction modes → chroma pred mode → CBP → residual. Each 4x4 block is
// predicted from already reconstructed neighbors (within the MB too), so the
// blocks go strictly in scan order.
func (sd *sliceDecoder) decodeMBNxN(addr int) error {
	r, f := sd.r, sd.frame
	mbX, mbY := addr%f.MbWidth, addr/f.MbWidth

	// High profile: transform_size_8x8_flag (CAVLC: u(1)) выбирает I_8x8 vs I_4x4.
	if sd.pps.Transform8x8Mode {
		if r.Flag() {
			sd.mbTransform8x8[addr] = true
			return sd.decodeMB8x8Cavlc(addr)
		}
	}

	// mb_pred: 16 Intra4x4PredMode.
	for blk := 0; blk < 16; blk++ {
		gx, gy := mbX*4+luma4x4BlockX[blk], mbY*4+luma4x4BlockY[blk]
		predMode := sd.predIntra4x4Mode(mbX, mbY, gx, gy)
		mode := predMode
		if !r.Flag() { // prev_intra4x4_pred_mode_flag == 0
			rem := int(r.U(3))
			if rem < predMode {
				mode = rem
			} else {
				mode = rem + 1
			}
		}
		sd.i4mode[gy*sd.w4+gx] = int8(mode)
	}

	chromaPredMode := 0
	if f.ChromaArrayType == 1 || f.ChromaArrayType == 2 {
		chromaPredMode = int(r.UE())
	}

	cbpLuma, cbpChroma, ok := readCBP(r, true)
	if !ok {
		return errors.New("h264: invalid coded_block_pattern")
	}
	if cbpLuma != 0 || cbpChroma != 0 {
		sd.qp = (sd.qp + int(r.SE()) + 52) % 52
	}
	qp := sd.qp

	// Luma: prediction + residual for each 4x4 block (scan order).
	for blk := 0; blk < 16; blk++ {
		bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
		gx, gy := mbX*4+bx, mbY*4+by

		top, left, corner, availTop, availLeft := sd.luma4x4Neighbors(gx, gy)
		var pred [16]byte
		predictIntra4x4(pred[:], int(sd.i4mode[gy*sd.w4+gx]), top, left, corner, availLeft, availTop)

		var d [16]int32
		var tc uint8
		if cbpLuma&(1<<uint(blk/4)) != 0 {
			n := sd.residualBlockCAVLC(r, sd.lumaNC(mbX, mbY, blk), 16)
			tc = uint8(n)
			sd.nzLuma[gy*sd.w4+gx] = tc
			d = h264k.DequantScan4x4(sd.coeffScratch[:16], qp)
			if n == 1 && sd.coeffScratch[0] != 0 {
				tc = 0 // DC-only block: take the IDCT DC shortcut
			}
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
		sd.reconstructChroma420(mbX, mbY, chromaPredMode, cbpChroma, qp)
	default:
		return ErrNotImplemented
	}
	return nil
}

// decodeMB8x8Cavlc decodes an I_NxN macroblock with transform_size_8x8_flag=1
// (High profile, CAVLC): 4 Intra8x8PredMode → chroma pred → CBP → 8x8 residual.
// Reconstruction reuses the 8x8 primitives (predictIntra8x8/inverseTransform8x8).
// In CAVLC the 8x8 luma residual is coded as 4 interleaved 4x4 blocks:
// coef8x8[4*i+j] = subblock_j[i] (spec 8.5.6). Chroma stays on the 4x4 path.
func (sd *sliceDecoder) decodeMB8x8Cavlc(addr int) error {
	r, f := sd.r, sd.frame
	mbX, mbY := addr%f.MbWidth, addr/f.MbWidth

	// mb_pred: 4 Intra8x8PredMode (записываем на все 4 под-4x4 каждого 8x8-блока).
	for blk8 := 0; blk8 < 4; blk8++ {
		gx, gy := mbX*4+(blk8%2)*2, mbY*4+(blk8/2)*2
		predMode := sd.predIntra4x4Mode(mbX, mbY, gx, gy)
		mode := predMode
		if !r.Flag() { // prev_intra8x8_pred_mode_flag == 0
			rem := int(r.U(3))
			if rem < predMode {
				mode = rem
			} else {
				mode = rem + 1
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
		chromaPredMode = int(r.UE())
	}

	cbpLuma, cbpChroma, ok := readCBP(r, true)
	if !ok {
		return errors.New("h264: invalid coded_block_pattern")
	}
	if cbpLuma != 0 || cbpChroma != 0 {
		sd.qp = (sd.qp + int(r.SE()) + 52) % 52
	}
	qp := sd.qp

	for blk8 := 0; blk8 < 4; blk8++ {
		b8x, b8y := blk8%2, blk8/2
		gx, gy := mbX*4+b8x*2, mbY*4+b8y*2
		px0, py0 := mbX*16+b8x*8, mbY*16+b8y*8

		top, left, corner, availTop, availLeft, availTL, _ := sd.luma8x8Neighbors(gx, gy, px0, py0)
		ft, fl, fc := filterRef8x8(top, left, corner, availTop, availLeft, availTL)
		var pred [64]byte
		predictIntra8x8(pred[:], int(sd.i4mode[gy*sd.w4+gx]), ft, fl, fc, availLeft, availTop)

		var res [64]int32
		if cbpLuma&(1<<uint(blk8)) != 0 {
			var coef [64]int32
			for j := 0; j < 4; j++ {
				blk := 4*blk8 + j // 4x4-блок blk8-квадранта в зиг-заг порядке
				n := sd.residualBlockCAVLC(r, sd.lumaNC(mbX, mbY, blk), 16)
				bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
				sd.nzLuma[(mbY*4+by)*sd.w4+mbX*4+bx] = uint8(n)
				scan := sd.coeffScratch[:16]
				for i := 0; i < 16; i++ {
					coef[4*i+j] = scan[i] // интерлив 4 под-блоков в 64 коэф.
				}
			}
			res = inverseTransform8x8(dequant8x8(inverseScan8x8(coef[:]), qp))
		} else {
			for j := 0; j < 4; j++ {
				blk := 4*blk8 + j
				bx, by := luma4x4BlockX[blk], luma4x4BlockY[blk]
				sd.nzLuma[(mbY*4+by)*sd.w4+mbX*4+bx] = 0
			}
		}
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				sd.recon4[(gy+dy)*sd.w4+gx+dx] = true
			}
		}
		h264k.IDCT8x8Add(f.Y, f.StrideY, px0, py0, pred[:], res)
	}

	switch f.ChromaArrayType {
	case 0:
	case 1:
		sd.reconstructChroma420(mbX, mbY, chromaPredMode, cbpChroma, qp)
	default:
		return ErrNotImplemented
	}
	return nil
}

// predIntra4x4Mode derives the predicted mode for a 4x4 block:
// min of the left/top neighbor modes; an unavailable neighbor or non-4x4 → DC(2).
func (sd *sliceDecoder) predIntra4x4Mode(mbX, mbY, gx, gy int) int {
	mw := sd.frame.MbWidth
	availA := gx > 0 && ((gx-1)/4 == mbX || sd.mbDecoded[mbY*mw+(gx-1)/4])
	availB := gy > 0 && ((gy-1)/4 == mbY || sd.mbDecoded[((gy-1)/4)*mw+mbX])
	if !availA || !availB {
		return 2 // dcPredModePredictedFlag
	}
	modeA, modeB := 2, 2
	if m := sd.i4mode[gy*sd.w4+gx-1]; m >= 0 {
		modeA = int(m)
	}
	if m := sd.i4mode[(gy-1)*sd.w4+gx]; m >= 0 {
		modeB = int(m)
	}
	return min(modeA, modeB)
}

// luma4x4Neighbors gathers neighbors for predicting a 4x4 block: top (8 samples,
// with top-right or the p[3,-1] substitution), left (4), corner. Availability is
// from the recon4 grid (accounts for the reconstruction order within the MB).
func (sd *sliceDecoder) luma4x4Neighbors(gx, gy int) (top, left []byte, corner byte, availTop, availLeft bool) {
	f := sd.frame
	top = sd.nTop4[:]
	left = sd.nLeft4[:]
	px0, py0 := gx*4, gy*4

	availTop = gy > 0 && sd.recon4[(gy-1)*sd.w4+gx]
	availLeft = gx > 0 && sd.recon4[gy*sd.w4+gx-1]
	availTL := gx > 0 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx-1]
	availTR := gx+1 < sd.w4 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx+1]

	if availTop {
		row := (py0 - 1) * f.StrideY
		for i := 0; i < 4; i++ {
			top[i] = f.Y[row+px0+i]
		}
	}
	if availTR {
		row := (py0 - 1) * f.StrideY
		for i := 0; i < 4; i++ {
			top[4+i] = f.Y[row+px0+4+i]
		}
	} else if availTop {
		for i := 0; i < 4; i++ {
			top[4+i] = top[3] // p[3,-1] substitution
		}
	}
	if availLeft {
		for i := 0; i < 4; i++ {
			left[i] = f.Y[(py0+i)*f.StrideY+px0-1]
		}
	}
	if availTL {
		corner = f.Y[(py0-1)*f.StrideY+px0-1]
	}
	return
}
