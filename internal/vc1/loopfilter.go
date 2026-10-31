package vc1

// loopFilterPlane applies the VC-1 in-loop deblocking filter to every
// internal 8x8 block edge of an I-picture plane: horizontal edges first, then
// vertical edges, in 4-sample segments decided by the third sample pair.
func loopFilterPlane(p []byte, stride, w, h, pq int) {
	for y := 8; y < h; y += 8 {
		for x := 0; x < w; x += 4 {
			loopFilterSegment(p, y*stride+x, 1, stride, pq)
		}
	}
	for x := 8; x < w; x += 8 {
		for y := 0; y < h; y += 4 {
			loopFilterSegment(p, y*stride+x, stride, 1, pq)
		}
	}
}

// loopFilterSegment filters four sample pairs along an edge. pos is the
// first sample past the edge, step moves along the edge and across moves
// across it.
func loopFilterSegment(p []byte, pos, step, across, pq int) {
	if filterLine(p, pos+2*step, across, pq) {
		filterLine(p, pos, across, pq)
		filterLine(p, pos+step, across, pq)
		filterLine(p, pos+3*step, across, pq)
	}
}

func filterLine(p []byte, i, s, pq int) bool {
	at := func(k int) int { return int(p[i+k*s]) }
	a0 := (2*(at(-2)-at(1)) - 5*(at(-1)-at(0)) + 4) >> 3
	a0Sign := a0 >> 31
	a0 = (a0 ^ a0Sign) - a0Sign
	if a0 >= pq {
		return false
	}
	a1 := absInt((2*(at(-4)-at(-1)) - 5*(at(-3)-at(-2)) + 4) >> 3)
	a2 := absInt((2*(at(0)-at(3)) - 5*(at(1)-at(2)) + 4) >> 3)
	if a1 >= a0 && a2 >= a0 {
		return false
	}
	clip := at(-1) - at(0)
	clipSign := clip >> 31
	clip = ((clip ^ clipSign) - clipSign) >> 1
	if clip == 0 {
		return false
	}
	a3 := min(a1, a2)
	dv := (5 * (a0 - a3)) >> 3
	if a0Sign^clipSign != 0 {
		dv = min(dv, clip)
		dv = (dv ^ clipSign) - clipSign
		p[i-s] = clipU8(at(-1) - dv)
		p[i] = clipU8(at(0) + dv)
	}
	return true
}
