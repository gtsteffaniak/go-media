package h264

// InvTransform4x4 applies H.264 4×4 inverse integer transform (ITU-T §8.5.10) in place.
// block is 16 coefficients in raster order; dst receives residual samples.
func InvTransform4x4(block []int16, dst []int16) {
	if len(block) < 16 || len(dst) < 16 {
		return
	}
	invTransform4x4Portable(block, dst)
}

func invTransform4x4Portable(block []int16, dst []int16) {
	var tmp [16]int16
	// Horizontal
	for i := 0; i < 4; i++ {
		e0 := block[i*4+0]
		e1 := block[i*4+1]
		e2 := block[i*4+2]
		e3 := block[i*4+3]
		f0 := e0 + e2
		f1 := e0 - e2
		f2 := (e1 >> 1) - e3
		f3 := e1 + (e3 >> 1)
		tmp[i*4+0] = f0 + f3
		tmp[i*4+1] = f1 + f2
		tmp[i*4+2] = f1 - f2
		tmp[i*4+3] = f0 - f3
	}
	// Vertical
	for j := 0; j < 4; j++ {
		e0 := tmp[0*4+j]
		e1 := tmp[1*4+j]
		e2 := tmp[2*4+j]
		e3 := tmp[3*4+j]
		f0 := e0 + e2
		f1 := e0 - e2
		f2 := (e1 >> 1) - e3
		f3 := e1 + (e3 >> 1)
		dst[0*4+j] = f0 + f3
		dst[1*4+j] = f1 + f2
		dst[2*4+j] = f1 - f2
		dst[3*4+j] = f0 - f3
	}
}

// AddPred4x4 adds residual to prediction plane at (x0,y0) with stride.
func AddPred4x4(pred, residual []int16, plane []byte, stride, x0, y0 int) {
	for y := 0; y < 4; y++ {
		row := (y0 + y) * stride
		for x := 0; x < 4; x++ {
			v := int(pred[y*4+x]) + int(residual[y*4+x])
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			plane[row+x0+x] = byte(v)
		}
	}
}
