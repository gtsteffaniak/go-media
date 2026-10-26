package h264

// WriteBlock4x4 adds a 4×4 prediction block and residual, clipping to 8-bit, into plane.
// predStride is the width of the prediction buffer; (lx, ly) is the block origin inside pred.
// res is raster-order int32 residuals from InverseTransform4x4Residual.
// WriteBlock4x4 adds prediction and residual for one 4×4 luma/chroma block.
func WriteBlock4x4(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, res [16]int32) {
	writeBlock4x4Portable(plane, stride, px0, py0, pred, predStride, lx, ly, res)
}

func writeBlock4x4Portable(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, res [16]int32) {
	for yy := 0; yy < 4; yy++ {
		row := (py0 + yy) * stride
		poff := (ly + yy) * predStride
		for xx := 0; xx < 4; xx++ {
			v := int(pred[poff+lx+xx]) + int(res[yy*4+xx])
			if v < 0 {
				v = 0
			} else if v > 255 {
				v = 255
			}
			plane[row+px0+xx] = byte(v)
		}
	}
}
