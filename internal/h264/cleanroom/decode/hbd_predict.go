package decode

// High-bit-depth (9/10-bit) intra predictors. These are scalar uint16 copies
// of the 8-bit predictors in predict.go, predict4x4.go and predict8x8.go with
// the sample range widened to [0, mx] (mx = (1<<BitDepth)-1) and the
// "no neighbors" DC value set to 1<<(BitDepth-1), as in spec 8.3. The 8-bit
// hot path does not call them.

// clipW clamps v to [0, mx].
func clipW(mx, v int) uint16 {
	if v < 0 {
		return 0
	}
	if v > mx {
		return uint16(mx)
	}
	return uint16(v)
}

func sumW(b []uint16) int {
	s := 0
	for _, v := range b {
		s += int(v)
	}
	return s
}

// predictIntra16x16At writes 16×16 intra prediction directly into the luma plane.
func predictIntra16x16AtW(mx int, plane []uint16, stride, px0, py0 int, mode int, top, left []uint16, corner uint16, availLeft, availTop bool) {
	switch mode {
	case I16Vertical:
		for y := 0; y < 16; y++ {
			row := (py0+y)*stride + px0
			for x := 0; x < 16; x++ {
				plane[row+x] = top[x]
			}
		}
	case I16Horizontal:
		for y := 0; y < 16; y++ {
			row := (py0+y)*stride + px0
			s := left[y]
			for x := 0; x < 16; x++ {
				plane[row+x] = s
			}
		}
	case I16DC:
		var dc uint16
		switch {
		case availTop && availLeft:
			dc = uint16((sumW(top) + sumW(left) + 16) >> 5)
		case availLeft:
			dc = uint16((sumW(left) + 8) >> 4)
		case availTop:
			dc = uint16((sumW(top) + 8) >> 4)
		default:
			dc = uint16((mx + 1) >> 1)
		}
		for y := 0; y < 16; y++ {
			row := (py0+y)*stride + px0
			for x := 0; x < 16; x++ {
				plane[row+x] = dc
			}
		}
	case I16Plane:
		h := 0
		for xp := 0; xp <= 7; xp++ {
			right := int(top[8+xp])
			var leftS int
			if xp < 7 {
				leftS = int(top[6-xp])
			} else {
				leftS = int(corner)
			}
			h += (xp + 1) * (right - leftS)
		}
		v := 0
		for yp := 0; yp <= 7; yp++ {
			lower := int(left[8+yp])
			var upper int
			if yp < 7 {
				upper = int(left[6-yp])
			} else {
				upper = int(corner)
			}
			v += (yp + 1) * (lower - upper)
		}
		b := (5*h + 32) >> 6
		c := (5*v + 32) >> 6
		a := 16 * (int(left[15]) + int(top[15]))
		for y := 0; y < 16; y++ {
			row := (py0+y)*stride + px0
			for x := 0; x < 16; x++ {
				plane[row+x] = clipW(mx, (a+b*(x-7)+c*(y-7)+16)>>5)
			}
		}
	}
}

// predictIntraChroma8x8At writes 8×8 chroma prediction into a plane at (px0,py0).
func predictIntraChroma8x8AtW(mx int, plane []uint16, stride, px0, py0 int, mode int, top, left []uint16, corner uint16, availLeft, availTop bool) {
	switch mode {
	case IChromaVertical:
		for y := 0; y < 8; y++ {
			row := (py0+y)*stride + px0
			for x := 0; x < 8; x++ {
				plane[row+x] = top[x]
			}
		}
	case IChromaHorizontal:
		for y := 0; y < 8; y++ {
			row := (py0+y)*stride + px0
			s := left[y]
			for x := 0; x < 8; x++ {
				plane[row+x] = s
			}
		}
	case IChromaDC:
		for by := 0; by < 8; by += 4 {
			for bx := 0; bx < 8; bx += 4 {
				dc := chromaDC4x4W(mx, top[bx:bx+4], left[by:by+4], bx, by, availLeft, availTop)
				for y := 0; y < 4; y++ {
					row := (py0+by+y)*stride + px0 + bx
					for x := 0; x < 4; x++ {
						plane[row+x] = dc
					}
				}
			}
		}
	case IChromaPlane:
		h := 0
		for xp := 0; xp <= 3; xp++ {
			right := int(top[4+xp])
			var leftS int
			if xp < 3 {
				leftS = int(top[2-xp])
			} else {
				leftS = int(corner)
			}
			h += (xp + 1) * (right - leftS)
		}
		v := 0
		for yp := 0; yp <= 3; yp++ {
			lower := int(left[4+yp])
			var upper int
			if yp < 3 {
				upper = int(left[2-yp])
			} else {
				upper = int(corner)
			}
			v += (yp + 1) * (lower - upper)
		}
		a := 16 * (int(left[7]) + int(top[7]))
		b := (34*h + 32) >> 6
		c := (34*v + 32) >> 6
		for y := 0; y < 8; y++ {
			row := (py0+y)*stride + px0
			for x := 0; x < 8; x++ {
				plane[row+x] = clipW(mx, (a+b*(x-3)+c*(y-3)+16)>>5)
			}
		}
	}
}

// chromaDC4x4 computes the DC for a single chroma 4x4 block, accounting for its
// position (mode 0). Corner blocks (0,0)/(4,4) average both directions;
// the top-right one prefers top, the bottom-left one prefers left.
func chromaDC4x4W(mx int, top4, left4 []uint16, bx, by int, availLeft, availTop bool) uint16 {
	useBoth := (bx == 0 && by == 0) || (bx > 0 && by > 0)
	preferTop := bx > 0 && by == 0
	switch {
	case useBoth:
		switch {
		case availTop && availLeft:
			return uint16((sumW(top4) + sumW(left4) + 4) >> 3)
		case availTop:
			return uint16((sumW(top4) + 2) >> 2)
		case availLeft:
			return uint16((sumW(left4) + 2) >> 2)
		}
	case preferTop:
		switch {
		case availTop:
			return uint16((sumW(top4) + 2) >> 2)
		case availLeft:
			return uint16((sumW(left4) + 2) >> 2)
		}
	default: // bottom-left: prefers left
		switch {
		case availLeft:
			return uint16((sumW(left4) + 2) >> 2)
		case availTop:
			return uint16((sumW(top4) + 2) >> 2)
		}
	}
	return uint16((mx + 1) >> 1)
}

// predictIntra4x4 fills dst (16 samples, stride 4) with the prediction of a 4x4 block.
func predictIntra4x4W(mx int, dst []uint16, mode int, top, left []uint16, corner uint16, availLeft, availTop bool) {
	// pt/pl: access p[i,-1]/p[-1,j] with support for index -1 (=corner).
	pt := func(i int) int {
		if i < 0 {
			return int(corner)
		}
		return int(top[i])
	}
	pl := func(j int) int {
		if j < 0 {
			return int(corner)
		}
		return int(left[j])
	}
	put := func(x, y, v int) { dst[y*4+x] = clipW(mx, v) }

	switch mode {
	case I4Vertical:
		for i := 0; i < 16; i++ {
			dst[i] = top[i&3]
		}
	case I4Horizontal:
		for i := 0; i < 16; i++ {
			dst[i] = left[i>>2]
		}
	case I4DC:
		var dc int
		switch {
		case availLeft && availTop:
			dc = (pt(0) + pt(1) + pt(2) + pt(3) + pl(0) + pl(1) + pl(2) + pl(3) + 4) >> 3
		case availLeft:
			dc = (pl(0) + pl(1) + pl(2) + pl(3) + 2) >> 2
		case availTop:
			dc = (pt(0) + pt(1) + pt(2) + pt(3) + 2) >> 2
		default:
			dc = (mx + 1) >> 1
		}
		for i := 0; i < 16; i++ {
			dst[i] = clipW(mx, dc)
		}
	case I4DiagDownLeft:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				if x == 3 && y == 3 {
					put(x, y, (pt(6)+3*pt(7)+2)>>2)
				} else {
					put(x, y, (pt(x+y)+2*pt(x+y+1)+pt(x+y+2)+2)>>2)
				}
			}
		}
	case I4DiagDownRight:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				switch {
				case x > y:
					put(x, y, (pt(x-y-2)+2*pt(x-y-1)+pt(x-y)+2)>>2)
				case x < y:
					put(x, y, (pl(y-x-2)+2*pl(y-x-1)+pl(y-x)+2)>>2)
				default:
					put(x, y, (pt(0)+2*int(corner)+pl(0)+2)>>2)
				}
			}
		}
	case I4VerticalRight:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				zVR := 2*x - y
				i := x - (y >> 1)
				switch {
				case zVR >= 0 && zVR%2 == 0:
					put(x, y, (pt(i-1)+pt(i)+1)>>1)
				case zVR >= 0:
					put(x, y, (pt(i-2)+2*pt(i-1)+pt(i)+2)>>2)
				case zVR == -1:
					put(x, y, (pl(0)+2*int(corner)+pt(0)+2)>>2)
				default:
					put(x, y, (pl(y-1)+2*pl(y-2)+pl(y-3)+2)>>2)
				}
			}
		}
	case I4HorizontalDown:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				zHD := 2*y - x
				j := y - (x >> 1)
				switch {
				case zHD >= 0 && zHD%2 == 0:
					put(x, y, (pl(j-1)+pl(j)+1)>>1)
				case zHD >= 0:
					put(x, y, (pl(j-2)+2*pl(j-1)+pl(j)+2)>>2)
				case zHD == -1:
					put(x, y, (pl(0)+2*int(corner)+pt(0)+2)>>2)
				default:
					put(x, y, (pt(x-1)+2*pt(x-2)+pt(x-3)+2)>>2)
				}
			}
		}
	case I4VerticalLeft:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				i := x + (y >> 1)
				if y%2 == 0 {
					put(x, y, (pt(i)+pt(i+1)+1)>>1)
				} else {
					put(x, y, (pt(i)+2*pt(i+1)+pt(i+2)+2)>>2)
				}
			}
		}
	case I4HorizontalUp:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				zHU := x + 2*y
				j := y + (x >> 1)
				switch {
				case zHU == 5:
					put(x, y, (pl(2)+3*pl(3)+2)>>2)
				case zHU > 5:
					put(x, y, pl(3))
				case zHU%2 == 0:
					put(x, y, (pl(j)+pl(j+1)+1)>>1)
				default:
					put(x, y, (pl(j)+2*pl(j+1)+pl(j+2)+2)>>2)
				}
			}
		}
	}
}

// filterRef8x8 low-pass filters the reference samples (the 8x8 prediction always
// uses filtered references). top is p[0..15,-1], left is p[-1,0..7], corner is
// p[-1,-1]. Returns the filtered ft[16], fl[8], fc.
func filterRef8x8W(top, left []uint16, corner uint16, availTop, availLeft, availTL bool) (ft [16]int, fl [8]int, fc int) {
	t := func(i int) int { return int(top[i]) }
	l := func(j int) int { return int(left[j]) }
	c := int(corner)

	if availTop {
		if availTL {
			ft[0] = (c + 2*t(0) + t(1) + 2) >> 2
		} else {
			ft[0] = (3*t(0) + t(1) + 2) >> 2
		}
		for x := 1; x <= 14; x++ {
			ft[x] = (t(x-1) + 2*t(x) + t(x+1) + 2) >> 2
		}
		ft[15] = (t(14) + 3*t(15) + 2) >> 2
	}
	if availLeft {
		if availTL {
			fl[0] = (c + 2*l(0) + l(1) + 2) >> 2
		} else {
			fl[0] = (3*l(0) + l(1) + 2) >> 2
		}
		for y := 1; y <= 6; y++ {
			fl[y] = (l(y-1) + 2*l(y) + l(y+1) + 2) >> 2
		}
		fl[7] = (l(6) + 3*l(7) + 2) >> 2
	}
	if availTL {
		switch {
		case availTop && availLeft:
			fc = (t(0) + 2*c + l(0) + 2) >> 2
		case availTop:
			fc = (3*c + t(0) + 2) >> 2
		case availLeft:
			fc = (3*c + l(0) + 2) >> 2
		}
	}
	return
}

// predictIntra8x8 fills dst (64 samples, stride 8) with the 8x8 prediction using
// the filtered references ft/fl/fc. Mode numbering matches the 4x4 modes.
func predictIntra8x8W(mx int, dst []uint16, mode int, ft [16]int, fl [8]int, fc int, availLeft, availTop bool) {
	pt := func(i int) int {
		if i < 0 {
			return fc
		}
		return ft[i]
	}
	pl := func(j int) int {
		if j < 0 {
			return fc
		}
		return fl[j]
	}
	put := func(x, y, v int) { dst[y*8+x] = clipW(mx, v) }

	switch mode {
	case I4Vertical:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				put(x, y, ft[x])
			}
		}
	case I4Horizontal:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				put(x, y, fl[y])
			}
		}
	case I4DC:
		var dc int
		switch {
		case availTop && availLeft:
			s := 8
			for i := 0; i < 8; i++ {
				s += ft[i] + fl[i]
			}
			dc = s >> 4
		case availLeft:
			s := 4
			for i := 0; i < 8; i++ {
				s += fl[i]
			}
			dc = s >> 3
		case availTop:
			s := 4
			for i := 0; i < 8; i++ {
				s += ft[i]
			}
			dc = s >> 3
		default:
			dc = (mx + 1) >> 1
		}
		for i := 0; i < 64; i++ {
			dst[i] = clipW(mx, dc)
		}
	case I4DiagDownLeft:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				if x == 7 && y == 7 {
					put(x, y, (ft[14]+3*ft[15]+2)>>2)
				} else {
					put(x, y, (pt(x+y)+2*pt(x+y+1)+pt(x+y+2)+2)>>2)
				}
			}
		}
	case I4DiagDownRight:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				switch {
				case x > y:
					put(x, y, (pt(x-y-2)+2*pt(x-y-1)+pt(x-y)+2)>>2)
				case x < y:
					put(x, y, (pl(y-x-2)+2*pl(y-x-1)+pl(y-x)+2)>>2)
				default:
					put(x, y, (pt(0)+2*fc+pl(0)+2)>>2)
				}
			}
		}
	case I4VerticalRight:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				zVR := 2*x - y
				i := x - (y >> 1)
				switch {
				case zVR >= 0 && zVR%2 == 0:
					put(x, y, (pt(i-1)+pt(i)+1)>>1)
				case zVR >= 0:
					put(x, y, (pt(i-2)+2*pt(i-1)+pt(i)+2)>>2)
				case zVR == -1:
					put(x, y, (pl(0)+2*fc+pt(0)+2)>>2)
				default:
					put(x, y, (pl(y-2*x-1)+2*pl(y-2*x-2)+pl(y-2*x-3)+2)>>2)
				}
			}
		}
	case I4HorizontalDown:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				zHD := 2*y - x
				j := y - (x >> 1)
				switch {
				case zHD >= 0 && zHD%2 == 0:
					put(x, y, (pl(j-1)+pl(j)+1)>>1)
				case zHD >= 0:
					put(x, y, (pl(j-2)+2*pl(j-1)+pl(j)+2)>>2)
				case zHD == -1:
					put(x, y, (pl(0)+2*fc+pt(0)+2)>>2)
				default:
					put(x, y, (pt(x-2*y-1)+2*pt(x-2*y-2)+pt(x-2*y-3)+2)>>2)
				}
			}
		}
	case I4VerticalLeft:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				i := x + (y >> 1)
				if y%2 == 0 {
					put(x, y, (pt(i)+pt(i+1)+1)>>1)
				} else {
					put(x, y, (pt(i)+2*pt(i+1)+pt(i+2)+2)>>2)
				}
			}
		}
	case I4HorizontalUp:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				zHU := x + 2*y
				j := y + (x >> 1)
				switch {
				case zHU == 13:
					put(x, y, (pl(6)+3*pl(7)+2)>>2)
				case zHU > 13:
					put(x, y, pl(7))
				case zHU%2 == 0:
					put(x, y, (pl(j)+pl(j+1)+1)>>1)
				default:
					put(x, y, (pl(j)+2*pl(j+1)+pl(j+2)+2)>>2)
				}
			}
		}
	}
}
