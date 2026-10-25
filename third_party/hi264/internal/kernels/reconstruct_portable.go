package kernels

func reconstructLuma4x4Portable(yPlane []byte, stride int, px, py int, pred [4][4]uint8, residual [16]int32) {
	for y := range 4 {
		row := (py+y)*stride + px
		for x := range 4 {
			v := int32(pred[y][x]) + residual[y*4+x]
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			yPlane[row+x] = byte(v)
		}
	}
}
