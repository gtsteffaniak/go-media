package kernels

func areaDownRowYPortable(dstY []byte, dx int, srcY []byte, srcStride int, sy0, sy1, x0, x1 int) {
	var ySum, n int
	for sy := sy0; sy < sy1; sy++ {
		row := sy * srcStride
		for sx := x0; sx < x1; sx++ {
			ySum += int(srcY[row+sx])
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	dstY[dx] = byte(ySum / n)
}

func areaDownChromaTilePortable(dstCb, dstCr []byte, dstCStride int, dx, dy int, srcCb, srcCr []byte, srcCStride int, sy0, sy1, x0, x1 int) {
	cy0 := sy0 / 2
	cy1 := (sy1 + 1) / 2
	cx0 := x0 / 2
	cx1 := (x1 + 1) / 2
	var cbSum, crSum, cn int
	for cy := cy0; cy < cy1; cy++ {
		row := cy * srcCStride
		for cx := cx0; cx < cx1; cx++ {
			cbSum += int(srcCb[row+cx])
			crSum += int(srcCr[row+cx])
			cn++
		}
	}
	if cn == 0 {
		cn = 1
	}
	ci := (dy/2)*dstCStride + (dx/2)
	dstCb[ci] = byte(cbSum / cn)
	dstCr[ci] = byte(crSum / cn)
}

func packRGBARowPortable(dst []byte, dstStride int, dy int, yy, cb, cr []byte) {
	row := dy * dstStride
	for dx := 0; dx < len(yy); dx++ {
		r, g, b := YCbCrToRGBFullRange(yy[dx], cb[dx], cr[dx])
		di := row + dx*4
		dst[di+0] = r
		dst[di+1] = g
		dst[di+2] = b
		dst[di+3] = 255
	}
}
