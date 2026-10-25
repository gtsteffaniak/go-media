package decode

// Luma 4x4 intra prediction — 9 modes. Inputs: top row
// top (p[0..7,-1], including top-right), left column left (p[-1,0..3]) and the
// corner corner (p[-1,-1]); output is the 4x4 block dst (row-major). Top-right
// substitution and neighbor availability are handled by the reconstruction layer.

const (
	I4Vertical       = 0
	I4Horizontal     = 1
	I4DC             = 2
	I4DiagDownLeft   = 3
	I4DiagDownRight  = 4
	I4VerticalRight  = 5
	I4HorizontalDown = 6
	I4VerticalLeft   = 7
	I4HorizontalUp   = 8
)

// predictIntra4x4 fills dst (16 samples, stride 4) with the prediction of a 4x4 block.
func predictIntra4x4(dst []byte, mode int, top, left []byte, corner byte, availLeft, availTop bool) {
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
	put := func(x, y, v int) { dst[y*4+x] = clip1(v) }

	switch mode {
	case I4Vertical:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				put(x, y, int(top[x]))
			}
		}
	case I4Horizontal:
		for y := 0; y < 4; y++ {
			for x := 0; x < 4; x++ {
				put(x, y, int(left[y]))
			}
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
			dc = 128
		}
		for i := 0; i < 16; i++ {
			dst[i] = clip1(dc)
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
