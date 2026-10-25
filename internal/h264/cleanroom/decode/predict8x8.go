package decode

// Intra 8x8 luma prediction for High profile (ISO/IEC 14496-10): reference
// sample filtering followed by the 9 prediction modes. Inputs are the raw
// neighbor samples; the top-right substitution is done by the reconstruction
// layer (as for 4x4).

// filterRef8x8 low-pass filters the reference samples (the 8x8 prediction always
// uses filtered references). top is p[0..15,-1], left is p[-1,0..7], corner is
// p[-1,-1]. Returns the filtered ft[16], fl[8], fc.
func filterRef8x8(top, left []byte, corner byte, availTop, availLeft, availTL bool) (ft [16]int, fl [8]int, fc int) {
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
func predictIntra8x8(dst []byte, mode int, ft [16]int, fl [8]int, fc int, availLeft, availTop bool) {
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
	put := func(x, y, v int) { dst[y*8+x] = clip1(v) }

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
			dc = 128
		}
		for i := 0; i < 64; i++ {
			dst[i] = clip1(dc)
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
