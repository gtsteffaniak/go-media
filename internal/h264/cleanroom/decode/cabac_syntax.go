package decode

// CABAC syntax-element decoding for I-slices (ISO/IEC 14496-10). Binarizations
// and context-index derivations are ported from OpenH264
// (parse_mb_syn_cabac.cpp / cabac_decoder.cpp; Cisco, BSD-2). The residual is
// handled separately; here are mb_type, intra prediction modes, delta_qp and the
// binarization primitives shared with the residual.

// ctxIdxOffset values (the NEW_CTX_OFFSET_* constants in OpenH264).
const (
	ctxMbTypeI    = 3
	ctxDeltaQP    = 60
	ctxChromaPred = 64 // intra_chroma_pred_mode
	ctxIntraPred  = 68 // prev_intra4x4_pred_mode_flag / rem
	ctxCbp        = 73 // coded_block_pattern
	ctxCbf        = 85 // coded_block_flag
	ctxSigMap     = 105
	ctxLastSig    = 166
	ctxCoeffOne   = 227
	ctxCoeffAbs   = 232
	ctxNumCbp     = 4 // contexts per chroma CBP bit group
)

// decodeUnary decodes a unary-binarized value: first bin at ctxIdx, subsequent
// bins at ctxIdx+ctxOffset, counting ones until a zero is read.
func (e *cabacEngine) decodeUnary(ctxIdx, ctxOffset int) int {
	if e.decodeDecision(ctxIdx) == 0 {
		return 0
	}
	sym := 0
	for {
		c := e.decodeDecision(ctxIdx + ctxOffset)
		sym++
		if c == 0 {
			return sym
		}
	}
}

// decodeExpBypass decodes an Exp-Golomb suffix using only bypass bins.
func (e *cabacEngine) decodeExpBypass(count int) int {
	symTmp := 0
	for {
		if e.decodeBypass() == 1 {
			symTmp += 1 << count
			count++
			if count == 16 {
				break
			}
		} else {
			break
		}
	}
	symTmp2 := 0
	for count > 0 {
		count--
		if e.decodeBypass() == 1 {
			symTmp2 |= 1 << count
		}
	}
	return symTmp + symTmp2
}

// decodeUEGLevel decodes the coeff_abs_level_minus1 suffix (UEG0): a truncated
// unary prefix (cMax 13) optionally followed by an Exp-Golomb bypass suffix.
func (e *cabacEngine) decodeUEGLevel(ctxIdx int) int {
	if e.decodeDecision(ctxIdx) == 0 {
		return 0
	}
	code, count := 0, 1
	var tmp uint32
	for {
		tmp = e.decodeDecision(ctxIdx)
		code++
		count++
		if tmp == 0 || count == 13 {
			break
		}
	}
	if tmp != 0 {
		code += e.decodeExpBypass(0) + 1
	}
	return code
}

// parseIMBTypeCabac decodes mb_type for an I-slice macroblock, returning the
// same 0..25 encoding used by decodeIMBType. leftNon4x4/topNon4x4 mean the
// neighbor MB is intra but not Intra_4x4/8x8 (or otherwise contributes to ctx).
func (e *cabacEngine) parseIMBTypeCabac(leftNon4x4, topNon4x4 bool) int {
	inc := b2i(leftNon4x4) + b2i(topNon4x4)
	if e.decodeDecision(ctxMbTypeI+inc) == 0 {
		return 0 // I_NxN
	}
	if e.decodeTerminate() == 1 {
		return 25 // I_PCM
	}
	mb := 1 // I_16x16
	if e.decodeDecision(ctxMbTypeI+3) == 1 {
		mb += 12 // CBPLuma != 0
	}
	if e.decodeDecision(ctxMbTypeI+4) == 1 {
		mb += 4
		if e.decodeDecision(ctxMbTypeI+5) == 1 {
			mb += 4
		}
	}
	mb += int(e.decodeDecision(ctxMbTypeI+6)) << 1
	mb += int(e.decodeDecision(ctxMbTypeI + 7))
	return mb
}

// parseIntraPredLumaCabac decodes prev_intra4x4_pred_mode_flag and, if zero, the
// 3-bit rem_intra4x4_pred_mode. Returns -1 when the predicted mode is used.
func (e *cabacEngine) parseIntraPredLumaCabac() int {
	if e.decodeDecision(ctxIntraPred) == 1 {
		return -1
	}
	v := int(e.decodeDecision(ctxIntraPred + 1))
	v |= int(e.decodeDecision(ctxIntraPred+1)) << 1
	v |= int(e.decodeDecision(ctxIntraPred+1)) << 2
	return v
}

// parseIntraChromaPredCabac decodes intra_chroma_pred_mode (0..3). inc is the
// neighbor-derived context increment for the first bin.
func (e *cabacEngine) parseIntraChromaPredCabac(inc int) int {
	if e.decodeDecision(ctxChromaPred+inc) == 0 {
		return 0
	}
	if e.decodeDecision(ctxChromaPred+3) == 0 {
		return 1
	}
	if e.decodeDecision(ctxChromaPred+3) == 0 {
		return 2
	}
	return 3
}

// parseDeltaQpCabac decodes mb_qp_delta. lastNonZero is whether the previous
// macroblock's mb_qp_delta was non-zero (the context increment for the first bin).
func (e *cabacEngine) parseDeltaQpCabac(lastNonZero bool) int {
	if e.decodeDecision(ctxDeltaQP+b2i(lastNonZero)) == 0 {
		return 0
	}
	code := e.decodeUnary(ctxDeltaQP+2, 1) + 1
	qpDelta := (code + 1) >> 1
	if code&1 == 0 {
		qpDelta = -qpDelta
	}
	return qpDelta
}
