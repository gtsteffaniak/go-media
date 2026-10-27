package decode

import h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"

// Intra prediction for H.264. The predictors are pure functions: they
// take neighboring reconstructed samples (top row top, left column left,
// corner corner) and availability flags, and produce the filled prediction
// block dst (row-major, stride = block width). The reconstruction layer gathers
// neighbors from the frame planes and then adds the residual.
//
// Currently implemented: luma I_16x16 (4 modes) and chroma 4:2:0 8x8 (4 modes).
// Luma 4x4 (I_NxN) and chroma 4:2:2/4:4:4 are the next slices.

// clip1 clamps the value to the 8-bit sample range [0, 255] (Clip1Y/C).
func clip1(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}

func sumBytes(b []byte) int {
	s := 0
	for _, v := range b {
		s += int(v)
	}
	return s
}

// Intra16x16 modes: 0=Vertical, 1=Horizontal, 2=DC, 3=Plane.
const (
	I16Vertical   = 0
	I16Horizontal = 1
	I16DC         = 2
	I16Plane      = 3
)

// predictIntra16x16 fills dst (256 samples, stride 16) with the prediction
// of a luma macroblock. top/left are 16 samples each, corner = p[-1,-1].
func predictIntra16x16(dst []byte, mode int, top, left []byte, corner byte, availLeft, availTop bool) {
	switch mode {
	case I16Vertical:
		h264k.PredictIntra16x16Vertical(dst, top)
	case I16Horizontal:
		h264k.PredictIntra16x16Horizontal(dst, left)
	case I16DC:
		var dc byte
		switch {
		case availTop && availLeft:
			dc = byte((sumBytes(top) + sumBytes(left) + 16) >> 5)
		case availLeft:
			dc = byte((sumBytes(left) + 8) >> 4)
		case availTop:
			dc = byte((sumBytes(top) + 8) >> 4)
		default:
			dc = 128
		}
		for i := range dst[:256] {
			dst[i] = dc
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
			for x := 0; x < 16; x++ {
				dst[y*16+x] = clip1((a + b*(x-7) + c*(y-7) + 16) >> 5)
			}
		}
	}
}

// predictIntra16x16At writes 16×16 intra prediction directly into the luma plane.
func predictIntra16x16At(plane []byte, stride, px0, py0 int, mode int, top, left []byte, corner byte, availLeft, availTop bool) {
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
		var dc byte
		switch {
		case availTop && availLeft:
			dc = byte((sumBytes(top) + sumBytes(left) + 16) >> 5)
		case availLeft:
			dc = byte((sumBytes(left) + 8) >> 4)
		case availTop:
			dc = byte((sumBytes(top) + 8) >> 4)
		default:
			dc = 128
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
				plane[row+x] = clip1((a + b*(x-7) + c*(y-7) + 16) >> 5)
			}
		}
	}
}

// Intra chroma modes: 0=DC, 1=Horizontal, 2=Vertical, 3=Plane.
const (
	IChromaDC         = 0
	IChromaHorizontal = 1
	IChromaVertical   = 2
	IChromaPlane      = 3
)

// predictIntraChroma8x8 fills dst (64 samples, stride 8) with the prediction
// of one chroma plane (4:2:0). top/left are 8 samples each, corner = p[-1,-1].
func predictIntraChroma8x8(dst []byte, mode int, top, left []byte, corner byte, availLeft, availTop bool) {
	switch mode {
	case IChromaVertical:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				dst[y*8+x] = top[x]
			}
		}
	case IChromaHorizontal:
		for y := 0; y < 8; y++ {
			for x := 0; x < 8; x++ {
				dst[y*8+x] = left[y]
			}
		}
	case IChromaDC:
		// DC is computed separately for each 4x4 block with a specific choice of neighbors.
		for by := 0; by < 8; by += 4 {
			for bx := 0; bx < 8; bx += 4 {
				dc := chromaDC4x4(top[bx:bx+4], left[by:by+4], bx, by, availLeft, availTop)
				for y := 0; y < 4; y++ {
					for x := 0; x < 4; x++ {
						dst[(by+y)*8+(bx+x)] = dc
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
			for x := 0; x < 8; x++ {
				dst[y*8+x] = clip1((a + b*(x-3) + c*(y-3) + 16) >> 5)
			}
		}
	}
}

// predictIntraChroma8x8At writes 8×8 chroma prediction into a plane at (px0,py0).
func predictIntraChroma8x8At(plane []byte, stride, px0, py0 int, mode int, top, left []byte, corner byte, availLeft, availTop bool) {
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
				dc := chromaDC4x4(top[bx:bx+4], left[by:by+4], bx, by, availLeft, availTop)
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
				plane[row+x] = clip1((a + b*(x-3) + c*(y-3) + 16) >> 5)
			}
		}
	}
}

// chromaDC4x4 computes the DC for a single chroma 4x4 block, accounting for its
// position (mode 0). Corner blocks (0,0)/(4,4) average both directions;
// the top-right one prefers top, the bottom-left one prefers left.
func chromaDC4x4(top4, left4 []byte, bx, by int, availLeft, availTop bool) byte {
	useBoth := (bx == 0 && by == 0) || (bx > 0 && by > 0)
	preferTop := bx > 0 && by == 0
	switch {
	case useBoth:
		switch {
		case availTop && availLeft:
			return byte((sumBytes(top4) + sumBytes(left4) + 4) >> 3)
		case availTop:
			return byte((sumBytes(top4) + 2) >> 2)
		case availLeft:
			return byte((sumBytes(left4) + 2) >> 2)
		}
	case preferTop:
		switch {
		case availTop:
			return byte((sumBytes(top4) + 2) >> 2)
		case availLeft:
			return byte((sumBytes(left4) + 2) >> 2)
		}
	default: // bottom-left: prefers left
		switch {
		case availLeft:
			return byte((sumBytes(left4) + 2) >> 2)
		case availTop:
			return byte((sumBytes(top4) + 2) >> 2)
		}
	}
	return 128
}
