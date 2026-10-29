package decode

// Deblocking filter. Currently for I-slices: boundary strength bS = 4 on
// macroblock edges and 3 on internal 4x4 edges (all intra). Luma and
// chroma 4:2:0. The threshold tables are ported from OpenH264
// (decoder/core/src/deblocking.cpp).

var alphaTable = [52]int{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	4, 4, 5, 6, 7, 8, 9, 10, 12, 13, 15, 17, 20, 22, 25, 28,
	32, 36, 40, 45, 50, 56, 63, 71, 80, 90, 101, 113, 127, 144, 162, 182,
	203, 226, 255, 255,
}
var betaTable = [52]int{
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 6, 6, 7, 7, 8, 8,
	9, 9, 10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 16, 16,
	17, 17, 18, 18,
}

// tc0Table[indexA][bS-1] for bS=1..3.
var tc0Table = [52][3]int{
	{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0},
	{0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0}, {0, 0, 0},
	{0, 0, 0}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 0, 1}, {0, 1, 1}, {0, 1, 1}, {1, 1, 1},
	{1, 1, 1}, {1, 1, 1}, {1, 1, 1}, {1, 1, 2}, {1, 1, 2}, {1, 1, 2}, {1, 1, 2}, {1, 2, 3},
	{1, 2, 3}, {2, 2, 3}, {2, 2, 4}, {2, 3, 4}, {2, 3, 4}, {3, 3, 5}, {3, 4, 6}, {3, 4, 6},
	{4, 5, 7}, {4, 5, 8}, {4, 6, 9}, {5, 7, 10}, {6, 8, 11}, {6, 8, 13}, {7, 10, 14}, {8, 11, 16},
	{9, 12, 18}, {10, 13, 20}, {11, 15, 23}, {13, 17, 25},
}

func clip3(lo, hi, v int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// deblock applies the filter to the whole frame (macroblocks in raster order).
func (sd *sliceDecoder) deblock() {
	if sd.h.DisableDeblock == 1 {
		return
	}
	for addr := 0; addr < sd.frame.MbWidth*sd.frame.MbHeight; addr++ {
		sd.deblockMB(addr%sd.frame.MbWidth, addr/sd.frame.MbWidth)
	}
}

func (sd *sliceDecoder) deblockMB(mbX, mbY int) {
	f := sd.frame
	cur := mbY*f.MbWidth + mbX
	qpCur := sd.mbQP[cur]
	offA, offB := int(sd.h.AlphaC0Offset), int(sd.h.BetaOffset)

	// thr computes alpha/beta/tc0 for an edge with side QPs qpP/qpQ.
	thr := func(qpP, qpQ, bS int) (alpha, beta, tc0 int) {
		qpav := (qpP + qpQ + 1) >> 1
		alpha = alphaTable[clip3(0, 51, qpav+offA)]
		beta = betaTable[clip3(0, 51, qpav+offB)]
		if bS < 4 {
			tc0 = tc0Table[clip3(0, 51, qpav+offA)][bS-1]
		}
		return
	}

	t8 := sd.mbTransform8x8 != nil && sd.mbTransform8x8[cur]

	// --- Vertical luma edges (filter horizontally, step 1) ---
	for e := 0; e < 4; e++ {
		if t8 && (e == 1 || e == 3) {
			continue // 8x8 transform: skip internal 4-sample edges
		}
		bS, qpP := 3, qpCur
		if e == 0 {
			if mbX == 0 {
				continue
			}
			bS, qpP = 4, sd.mbQP[cur-1]
		}
		alpha, beta, tc0 := thr(qpP, qpCur, bS)
		if alpha == 0 {
			continue
		}
		x := mbX*16 + e*4
		for y := 0; y < 16; y++ {
			filterLuma(f.Y, (mbY*16+y)*f.StrideY+x, 1, bS, alpha, beta, tc0)
		}
	}
	// --- Vertical chroma 4:2:0 edges (x in {0,4}) ---
	if f.ChromaArrayType == 1 {
		for ce := 0; ce < 2; ce++ {
			bS, qpP := 3, qpCur
			if ce == 0 {
				if mbX == 0 {
					continue
				}
				bS, qpP = 4, sd.mbQP[cur-1]
			}
			sd.filterChromaEdge(mbX, mbY, ce*4, 0, 1, bS, qpP, qpCur, true)
		}
	}

	// --- Horizontal luma edges (step = StrideY) ---
	for e := 0; e < 4; e++ {
		if t8 && (e == 1 || e == 3) {
			continue // 8x8 transform: skip internal 4-sample edges
		}
		bS, qpP := 3, qpCur
		if e == 0 {
			if mbY == 0 {
				continue
			}
			bS, qpP = 4, sd.mbQP[cur-f.MbWidth]
		}
		alpha, beta, tc0 := thr(qpP, qpCur, bS)
		if alpha == 0 {
			continue
		}
		y := mbY*16 + e*4
		for x := 0; x < 16; x++ {
			filterLuma(f.Y, y*f.StrideY+mbX*16+x, f.StrideY, bS, alpha, beta, tc0)
		}
	}
	// --- Horizontal chroma 4:2:0 edges (y in {0,4}) ---
	if f.ChromaArrayType == 1 {
		for ce := 0; ce < 2; ce++ {
			bS, qpP := 3, qpCur
			if ce == 0 {
				if mbY == 0 {
					continue
				}
				bS, qpP = 4, sd.mbQP[cur-f.MbWidth]
			}
			sd.filterChromaEdge(mbX, mbY, 0, ce*4, f.StrideC, bS, qpP, qpCur, false)
		}
	}
}

// filterChromaEdge filters one chroma edge across both components. (cx,cy) is
// the edge offset in chroma samples from the top-left of the MB; step=1 for
// vertical, =StrideC for horizontal; vertical selects the traversal axis.
func (sd *sliceDecoder) filterChromaEdge(mbX, mbY, cx, cy, step, bS, qpP, qpQ int, vertical bool) {
	f := sd.frame
	offA, offB := int(sd.h.AlphaC0Offset), int(sd.h.BetaOffset)
	x0, y0 := mbX*f.MbWidthC+cx, mbY*f.MbHeightC+cy
	// Cb and Cr use different QP offsets, so each plane derives its own
	// qPav, alpha, beta, and tc0 (8.7.2.4).
	for comp, plane := range [][]byte{f.Cb, f.Cr} {
		qpPc := chromaQPFor(sd.pps, comp, qpP)
		qpQc := chromaQPFor(sd.pps, comp, qpQ)
		qpav := (qpPc + qpQc + 1) >> 1
		alpha := alphaTable[clip3(0, 51, qpav+offA)]
		beta := betaTable[clip3(0, 51, qpav+offB)]
		if alpha == 0 {
			continue
		}
		tc0 := 0
		if bS < 4 {
			tc0 = tc0Table[clip3(0, 51, qpav+offA)][bS-1]
		}
		for i := 0; i < 8; i++ {
			var idx int
			if vertical {
				idx = (y0+i)*f.StrideC + x0
			} else {
				idx = y0*f.StrideC + x0 + i
			}
			filterChroma(plane, idx, step, bS, alpha, beta, tc0)
		}
	}
}

// filterLuma filters one luma line across the edge.
// q0idx is the index of q0; step is the stride to neighboring samples (1 or a row).
func filterLuma(plane []byte, q0idx, step, bS, alpha, beta, tc0 int) {
	p0 := int(plane[q0idx-step])
	p1 := int(plane[q0idx-2*step])
	p2 := int(plane[q0idx-3*step])
	p3 := int(plane[q0idx-4*step])
	q0 := int(plane[q0idx])
	q1 := int(plane[q0idx+step])
	q2 := int(plane[q0idx+2*step])
	q3 := int(plane[q0idx+3*step])
	if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
		return
	}
	ap, aq := absInt(p2-p0), absInt(q2-q0)
	if bS == 4 {
		if ap < beta && absInt(p0-q0) < (alpha>>2)+2 {
			plane[q0idx-step] = clip1((p2 + 2*p1 + 2*p0 + 2*q0 + q1 + 4) >> 3)
			plane[q0idx-2*step] = clip1((p2 + p1 + p0 + q0 + 2) >> 2)
			plane[q0idx-3*step] = clip1((2*p3 + 3*p2 + p1 + p0 + q0 + 4) >> 3)
		} else {
			plane[q0idx-step] = clip1((2*p1 + p0 + q1 + 2) >> 2)
		}
		if aq < beta && absInt(p0-q0) < (alpha>>2)+2 {
			plane[q0idx] = clip1((q2 + 2*q1 + 2*q0 + 2*p0 + p1 + 4) >> 3)
			plane[q0idx+step] = clip1((q2 + q1 + q0 + p0 + 2) >> 2)
			plane[q0idx+2*step] = clip1((2*q3 + 3*q2 + q1 + q0 + p0 + 4) >> 3)
		} else {
			plane[q0idx] = clip1((2*q1 + q0 + p1 + 2) >> 2)
		}
		return
	}
	tc := tc0 + b2i(ap < beta) + b2i(aq < beta)
	delta := clip3(-tc, tc, ((q0-p0)<<2+(p1-q1)+4)>>3)
	plane[q0idx-step] = clip1(p0 + delta)
	plane[q0idx] = clip1(q0 - delta)
	if ap < beta {
		plane[q0idx-2*step] = clip1(p1 + clip3(-tc0, tc0, (p2+((p0+q0+1)>>1)-2*p1)>>1))
	}
	if aq < beta {
		plane[q0idx+step] = clip1(q1 + clip3(-tc0, tc0, (q2+((p0+q0+1)>>1)-2*q1)>>1))
	}
}

// filterChroma filters one chroma line (only p0,q0 are modified).
func filterChroma(plane []byte, q0idx, step, bS, alpha, beta, tc0 int) {
	p0 := int(plane[q0idx-step])
	p1 := int(plane[q0idx-2*step])
	q0 := int(plane[q0idx])
	q1 := int(plane[q0idx+step])
	if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
		return
	}
	if bS == 4 {
		plane[q0idx-step] = clip1((2*p1 + p0 + q1 + 2) >> 2)
		plane[q0idx] = clip1((2*q1 + q0 + p1 + 2) >> 2)
		return
	}
	tc := tc0 + 1
	delta := clip3(-tc, tc, ((q0-p0)<<2+(p1-q1)+4)>>3)
	plane[q0idx-step] = clip1(p0 + delta)
	plane[q0idx] = clip1(q0 - delta)
}
