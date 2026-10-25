package decode

// CABAC residual decoding: coded_block_flag, the significance map and the
// coefficient levels. Returns coefficients in scan order, exactly like the
// CAVLC path, so the dequant/transform/reconstruction code is reused unchanged.
// Context tables and logic are ported from OpenH264 (parse_mb_syn_cabac.cpp;
// Cisco, BSD-2).
//
// ResProperty values used here (block categories): I16_LUMA_DC=1, I16_LUMA_AC=2,
// LUMA_4x4=3, CHROMA_DC_U=7, CHROMA_DC_V=8, CHROMA_AC_U=9, CHROMA_AC_V=10.

const (
	resI16LumaDC = 1
	resI16LumaAC = 2
	resLuma4x4   = 3
	resLumaDC8   = 6 // LUMA_DC_AC_8 (8x8 luma block)
	resChromaDCU = 7
	resChromaDCV = 8
	resChromaACU = 9
	resChromaACV = 10
)

// CABAC context offsets for the 8x8 residual (OpenH264 NEW_CTX_OFFSET_*_8x8).
const (
	ctxTransform8x8 = 399
	ctxSigMap8x8    = 402
	ctxLast8x8      = 417
	ctxOne8x8       = 426
	ctxAbs8x8       = 431
)

// significant_coeff_flag / last_significant_coeff_flag context index maps for
// 8x8 blocks (ported from OpenH264 g_kuiIdx2Ctx*SignificantCoeffFlag8x8).
var sig8x8Map = [64]int{
	0, 1, 2, 3, 4, 5, 5, 4, 4, 3, 3, 4, 4, 4, 5, 5,
	4, 4, 4, 4, 3, 3, 6, 7, 7, 7, 8, 9, 10, 9, 8, 7,
	7, 6, 11, 12, 13, 11, 6, 7, 8, 9, 14, 10, 9, 8, 6, 11,
	12, 13, 11, 6, 9, 14, 10, 9, 11, 12, 13, 11, 14, 10, 12, 14,
}
var last8x8Map = [64]int{
	0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	3, 3, 3, 3, 3, 3, 3, 3, 4, 4, 4, 4, 4, 4, 4, 4,
	5, 5, 5, 5, 6, 6, 6, 6, 7, 7, 7, 7, 8, 8, 8, 8,
}

// Per-ResProperty tables, indexed 1..10 (entry 0 and 6 = 8x8 are unused here).
var (
	blockCatMaxPos  = [11]int{0, 15, 14, 15, 3, 14, 63, 3, 3, 14, 14}
	blockCatMaxC2   = [11]int{0, 4, 4, 4, 3, 4, 4, 3, 3, 4, 4}
	blockCat2CBF    = [11]int{0, 0, 4, 8, 12, 16, 0, 12, 12, 16, 16}
	blockCat2Map    = [11]int{0, 0, 15, 29, 44, 47, 0, 44, 44, 47, 47}
	blockCat2Last   = [11]int{0, 0, 15, 29, 44, 47, 0, 44, 44, 47, 47}
	blockCat2One    = [11]int{0, 0, 10, 20, 30, 39, 0, 30, 30, 39, 39}
	blockCat2AbsTab = [11]int{0, 0, 10, 20, 30, 39, 0, 30, 30, 39, 39}
)

// residualBlockCABAC decodes one residual block via CABAC and returns the
// coefficients in scan order plus the number of nonzero coefficients. condA/condB
// are the coded_block_flag context increments derived from the neighbor blocks.
func residualBlockCABAC(sd *sliceDecoder, resProp, condA, condB int) ([]int32, int) {
	e := sd.cb
	maxPos := blockCatMaxPos[resProp]
	if maxPos+1 > len(sd.resCoeff) {
		panic("residualBlockCABAC: block too large for scratch")
	}
	coeff := sd.resCoeff[:maxPos+1]
	for i := range coeff {
		coeff[i] = 0
	}
	is8x8 := resProp == resLumaDC8

	// The 8x8 luma block has no coded_block_flag (presence comes from CBP).
	if !is8x8 {
		cbfCtx := ctxCbf + blockCat2CBF[resProp] + condA + 2*condB
		if e.decodeDecision(cbfCtx) == 0 {
			return coeff, 0
		}
	}

	// Significance map: 1 at significant scan positions.
	sig := sd.resSig[:maxPos+1]
	for i := range sig {
		sig[i] = false
	}
	coeffNum := 0
	mapBase, lastBase := ctxSigMap+blockCat2Map[resProp], ctxLastSig+blockCat2Last[resProp]
	if is8x8 {
		mapBase, lastBase = ctxSigMap8x8, ctxLast8x8
	}
	sigCtx := func(i int) int {
		if is8x8 {
			return sig8x8Map[i]
		}
		return i
	}
	lastCtx := func(i int) int {
		if is8x8 {
			return last8x8Map[i]
		}
		return i
	}
	foundLast := false
	for i := 0; i < maxPos; i++ {
		if e.decodeDecision(mapBase+sigCtx(i)) == 1 {
			sig[i] = true
			coeffNum++
			if e.decodeDecision(lastBase+lastCtx(i)) == 1 {
				foundLast = true
				break
			}
		}
	}
	if !foundLast {
		sig[maxPos] = true
		coeffNum++
	}

	// Coefficient levels, decoded in reverse scan order.
	oneBase, absBase := ctxCoeffOne+blockCat2One[resProp], ctxCoeffAbs+blockCat2AbsTab[resProp]
	if is8x8 {
		oneBase, absBase = ctxOne8x8, ctxAbs8x8
	}
	maxC2 := blockCatMaxC2[resProp]
	c1, c2 := 1, 0
	for p := maxPos; p >= 0; p-- {
		if !sig[p] {
			continue
		}
		level := 1
		if e.decodeDecision(oneBase+c1) == 1 {
			level = 2 + e.decodeUEGLevel(absBase+c2)
			c2++
			if c2 > maxC2 {
				c2 = maxC2
			}
			c1 = 0
		} else if c1 != 0 {
			c1++
			if c1 > 4 {
				c1 = 4
			}
		}
		if e.decodeBypass() == 1 {
			level = -level
		}
		coeff[p] = int32(level)
	}
	return coeff, coeffNum
}
