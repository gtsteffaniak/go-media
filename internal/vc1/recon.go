package vc1

func (d *intraDecoder) putBlocksDelayed(signed bool) {
	if !d.firstSliceLn {
		if d.mbX > 0 {
			d.putFromRing(d.blkRing[d.blkTopLeft], d.mbX-1, d.mbY-1, signed)
		}
		if d.mbX == d.endMBX-1 {
			d.putFromRing(d.blkRing[d.blkTop], d.mbX, d.mbY-1, signed)
		}
	}
	if d.mbY == d.mbh-1 {
		if d.mbX > 0 {
			d.putFromRing(d.blkRing[d.blkLeft], d.mbX-1, d.mbY, signed)
		}
		if d.mbX == d.endMBX-1 {
			d.putFromRing(d.blkRing[d.blkCur], d.mbX, d.mbY, signed)
		}
	}
}

func (d *intraDecoder) putFromRing(blocks [][64]int16, mbx, mby int, signed bool) {
	if mbx < 0 || mby < 0 || mbx >= d.mbw || mby >= d.mbh {
		return
	}
	for i := 0; i < 6; i++ {
		b := blocks[blockMap[i]]
		if i < 4 {
			off := d.xPad + mbx*16 + (i&1)*8 + (d.yPad+mby*16+(i>>1)*8)*d.yBufStride
			putPixels8x8(d.destY[off:], d.yBufStride, b, signed)
		} else if i == 4 {
			off := d.xPad + mbx*8 + (d.yPad+mby*8)*d.cBufStride
			putPixels8x8(d.destCb[off:], d.cBufStride, b, signed)
		} else {
			off := d.xPad + mbx*8 + (d.yPad+mby*8)*d.cBufStride
			putPixels8x8(d.destCr[off:], d.cBufStride, b, signed)
		}
	}
}

func putPixels8x8(dest []byte, stride int, block [64]int16, signed bool) {
	for y := 0; y < 8; y++ {
		row := dest[y*stride:]
		for x := 0; x < 8; x++ {
			v := int(block[y*8+x])
			if !signed {
				v += 128
			}
			row[x] = clipU8(v)
		}
	}
}

func (d *intraDecoder) iOverlapMB(cur [][64]int16) {
	for i := 0; i < 6; i++ {
		if d.mbX == 0 && (i&5) != 1 {
			continue
		}
		if d.mbX > 0 {
			hOverlapInt16(d.blkRing[d.blkLeft][blockMap[i]], cur[blockMap[i]], 0)
		}
	}
	if !d.firstSliceLn && d.mbX > 0 {
		vOverlapInt16(d.blkRing[d.blkTop][blockMap[0]], cur[blockMap[0]])
	}
}

func hOverlapInt16(left, right [64]int16, flags int) {
	rnd1 := 4
	if flags&2 != 0 {
		rnd1 = 3
	}
	rnd2 := 7 - rnd1
	for row := 0; row < 8; row++ {
		base := row * 8
		a, b, c, vd := left[base+6], left[base+7], right[base+0], right[base+1]
		d1 := int(a) - int(vd)
		d2 := d1 + int(b) - int(c)
		left[base+6] = int16((int(a)*8 - d1 + rnd1) >> 3)
		left[base+7] = int16((int(b)*8 - d2 + rnd2) >> 3)
		right[base+0] = int16((int(c)*8 + d2 + rnd1) >> 3)
		right[base+1] = int16((int(vd)*8 + d1 + rnd2) >> 3)
	}
}

func vOverlapInt16(top, bottom [64]int16) {
	rnd1, rnd2 := 4, 3
	for col := 0; col < 8; col++ {
		a, b, c, vd := top[48+col], top[56+col], bottom[col], bottom[8+col]
		d1 := int(a) - int(vd)
		d2 := d1 + int(b) - int(c)
		top[48+col] = int16((int(a)*8 - d1 + rnd1) >> 3)
		top[56+col] = int16((int(b)*8 - d2 + rnd2) >> 3)
		bottom[col] = int16((int(c)*8 + d2 + rnd1) >> 3)
		bottom[8+col] = int16((int(vd)*8 + d1 + rnd2) >> 3)
		rnd2 = 7 - rnd2
		rnd1 = 7 - rnd1
	}
}
