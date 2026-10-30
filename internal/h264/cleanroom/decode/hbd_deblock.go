package decode

// High-bit-depth deblocking: scalar uint16 copies of deblockMB and the edge
// filters in deblock.go. Per spec 8.7.2.2 the alpha, beta and tC0 table
// values are scaled by 1<<(BitDepth-8); qPav is taken from QPY (which may be
// negative down to -QpBdOffsetY), and chroma QPs come from chromaQPW.

// deblockW applies the in-loop filter to the high-bit-depth planes.
func (sd *sliceDecoder) deblockW() {
	if sd.h.DisableDeblock == 1 {
		return
	}
	for addr := 0; addr < sd.frame.MbWidth*sd.frame.MbHeight; addr++ {
		sd.deblockMBW(addr%sd.frame.MbWidth, addr/sd.frame.MbWidth)
	}
}

func (sd *sliceDecoder) deblockMBW(mbX, mbY int) {
	f := sd.frame
	cur := mbY*f.MbWidth + mbX
	qpCur := sd.mbQP[cur]
	offA, offB := int(sd.h.AlphaC0Offset), int(sd.h.BetaOffset)
	sh, mx := uint(f.BitDepthY-8), (1<<f.BitDepthY)-1

	// thr computes alpha/beta/tc0 for an edge with side QPs qpP/qpQ.
	thr := func(qpP, qpQ, bS int) (alpha, beta, tc0 int) {
		qpav := (qpP + qpQ + 1) >> 1
		alpha = alphaTable[clip3(0, 51, qpav+offA)] << sh
		beta = betaTable[clip3(0, 51, qpav+offB)] << sh
		if bS < 4 {
			tc0 = tc0Table[clip3(0, 51, qpav+offA)][bS-1] << sh
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
			filterLumaW(mx, f.Y16, (mbY*16+y)*f.StrideY+x, 1, bS, alpha, beta, tc0)
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
			sd.filterChromaEdgeW(mbX, mbY, ce*4, 0, 1, bS, qpP, qpCur, true)
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
			filterLumaW(mx, f.Y16, y*f.StrideY+mbX*16+x, f.StrideY, bS, alpha, beta, tc0)
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
			sd.filterChromaEdgeW(mbX, mbY, 0, ce*4, f.StrideC, bS, qpP, qpCur, false)
		}
	}
}

func (sd *sliceDecoder) filterChromaEdgeW(mbX, mbY, cx, cy, step, bS, qpP, qpQ int, vertical bool) {
	f := sd.frame
	offA, offB := int(sd.h.AlphaC0Offset), int(sd.h.BetaOffset)
	sh, mx := uint(f.BitDepthC-8), (1<<f.BitDepthC)-1
	x0, y0 := mbX*f.MbWidthC+cx, mbY*f.MbHeightC+cy
	// Cb and Cr use different QP offsets, so each plane derives its own
	// qPav, alpha, beta, and tc0 (8.7.2.4).
	for comp, plane := range [][]uint16{f.Cb16, f.Cr16} {
		qpPc := sd.chromaQPW(comp, qpP)
		qpQc := sd.chromaQPW(comp, qpQ)
		qpav := (qpPc + qpQc + 1) >> 1
		alpha := alphaTable[clip3(0, 51, qpav+offA)] << sh
		beta := betaTable[clip3(0, 51, qpav+offB)] << sh
		if alpha == 0 {
			continue
		}
		tc0 := 0
		if bS < 4 {
			tc0 = tc0Table[clip3(0, 51, qpav+offA)][bS-1] << sh
		}
		for i := 0; i < 8; i++ {
			var idx int
			if vertical {
				idx = (y0+i)*f.StrideC + x0
			} else {
				idx = y0*f.StrideC + x0 + i
			}
			filterChromaW(mx, plane, idx, step, bS, alpha, beta, tc0)
		}
	}
}

func filterLumaW(mx int, plane []uint16, q0idx, step, bS, alpha, beta, tc0 int) {
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
			plane[q0idx-step] = clipW(mx, (p2+2*p1+2*p0+2*q0+q1+4)>>3)
			plane[q0idx-2*step] = clipW(mx, (p2+p1+p0+q0+2)>>2)
			plane[q0idx-3*step] = clipW(mx, (2*p3+3*p2+p1+p0+q0+4)>>3)
		} else {
			plane[q0idx-step] = clipW(mx, (2*p1+p0+q1+2)>>2)
		}
		if aq < beta && absInt(p0-q0) < (alpha>>2)+2 {
			plane[q0idx] = clipW(mx, (q2+2*q1+2*q0+2*p0+p1+4)>>3)
			plane[q0idx+step] = clipW(mx, (q2+q1+q0+p0+2)>>2)
			plane[q0idx+2*step] = clipW(mx, (2*q3+3*q2+q1+q0+p0+4)>>3)
		} else {
			plane[q0idx] = clipW(mx, (2*q1+q0+p1+2)>>2)
		}
		return
	}
	tc := tc0 + b2i(ap < beta) + b2i(aq < beta)
	delta := clip3(-tc, tc, ((q0-p0)<<2+(p1-q1)+4)>>3)
	plane[q0idx-step] = clipW(mx, p0+delta)
	plane[q0idx] = clipW(mx, q0-delta)
	if ap < beta {
		plane[q0idx-2*step] = clipW(mx, p1+clip3(-tc0, tc0, (p2+((p0+q0+1)>>1)-2*p1)>>1))
	}
	if aq < beta {
		plane[q0idx+step] = clipW(mx, q1+clip3(-tc0, tc0, (q2+((p0+q0+1)>>1)-2*q1)>>1))
	}
}

func filterChromaW(mx int, plane []uint16, q0idx, step, bS, alpha, beta, tc0 int) {
	p0 := int(plane[q0idx-step])
	p1 := int(plane[q0idx-2*step])
	q0 := int(plane[q0idx])
	q1 := int(plane[q0idx+step])
	if absInt(p0-q0) >= alpha || absInt(p1-p0) >= beta || absInt(q1-q0) >= beta {
		return
	}
	if bS == 4 {
		plane[q0idx-step] = clipW(mx, (2*p1+p0+q1+2)>>2)
		plane[q0idx] = clipW(mx, (2*q1+q0+p1+2)>>2)
		return
	}
	tc := tc0 + 1
	delta := clip3(-tc, tc, ((q0-p0)<<2+(p1-q1)+4)>>3)
	plane[q0idx-step] = clipW(mx, p0+delta)
	plane[q0idx] = clipW(mx, q0-delta)
}
