package vc1

const (
	edgeLeft   = 1 << 0
	edgeRight  = 1 << 1
	edgeTop    = 1 << 2
	edgeBottom = 1 << 3
)

func (d *intraDecoder) iLoopFilter() {
	blockCount := 6
	if !d.firstSliceLn {
		flags := edgeTop
		if d.mbX > 0 {
			mbx, mby := d.mbX-1, d.mbY-1
			for i := 0; i < blockCount; i++ {
				d.iVLoopFilter(mbx, mby, flags, i)
			}
		}
		if d.mbX == d.endMBX-1 {
			mbx, mby := d.mbX, d.mbY-1
			for i := 0; i < blockCount; i++ {
				d.iVLoopFilter(mbx, mby, flags, i)
			}
		}
	}
	if d.mbY == d.mbh-1 {
		flags := edgeBottom
		if d.firstSliceLn {
			flags |= edgeTop
		}
		if d.mbX > 0 {
			mbx, mby := d.mbX-1, d.mbY
			for i := 0; i < blockCount; i++ {
				d.iVLoopFilter(mbx, mby, flags, i)
			}
		}
		if d.mbX == d.endMBX-1 {
			mbx, mby := d.mbX, d.mbY
			for i := 0; i < blockCount; i++ {
				d.iVLoopFilter(mbx, mby, flags, i)
			}
		}
	}
	if d.mbY >= 2 {
		if d.mbX > 0 {
			mbx, mby := d.mbX-1, d.mbY-2
			for i := 0; i < blockCount; i++ {
				d.iHLoopFilter(mbx, mby, edgeLeft, i)
			}
		}
		if d.mbX == d.endMBX-1 {
			flags := edgeRight
			if d.mbX == 0 {
				flags = edgeLeft | edgeRight
			}
			mbx, mby := d.mbX, d.mbY-2
			for i := 0; i < blockCount; i++ {
				d.iHLoopFilter(mbx, mby, flags, i)
			}
		}
	}
	if d.mbY == d.mbh-1 {
		if d.mbY >= 1 {
			if d.mbX > 0 {
				mbx, mby := d.mbX-1, d.mbY-1
				for i := 0; i < blockCount; i++ {
					d.iHLoopFilter(mbx, mby, edgeLeft, i)
				}
			}
			if d.mbX == d.endMBX-1 {
				flags := edgeRight
				if d.mbX == 0 {
					flags = edgeLeft | edgeRight
				}
				mbx, mby := d.mbX, d.mbY-1
				for i := 0; i < blockCount; i++ {
					d.iHLoopFilter(mbx, mby, flags, i)
				}
			}
		}
		if d.mbX > 0 {
			mbx, mby := d.mbX-1, d.mbY
			for i := 0; i < blockCount; i++ {
				d.iHLoopFilter(mbx, mby, edgeLeft, i)
			}
		}
		if d.mbX == d.endMBX-1 {
			mbx, mby := d.mbX, d.mbY
			for i := 0; i < blockCount; i++ {
				d.iHLoopFilter(mbx, mby, edgeRight, i)
			}
		}
	}
}

func (d *intraDecoder) yBlockBase(mbx, mby, blockNum int) int {
	base := d.xPad + mbx*16 + (d.yPad+mby*16)*d.yBufStride
	if blockNum <= 3 {
		base += (blockNum&2)*4*d.yBufStride + (blockNum&1)*8
	}
	return base
}

func (d *intraDecoder) cBlockBase(mbx, mby int) int {
	return d.xPad + mbx*8 + (d.yPad+mby*8)*d.cBufStride
}

func (d *intraDecoder) iHLoopFilter(mbx, mby int, flags int, blockNum int) {
	if blockNum&2 != 0 {
		return
	}
	if flags&edgeLeft != 0 && (blockNum&5) == 1 {
		return
	}
	if blockNum <= 3 {
		vc1LoopFilter(d.destY, d.yBlockBase(mbx, mby, blockNum), d.yBufStride, 1, 16, d.pq)
		return
	}
	plane := d.destCb
	if blockNum != 4 {
		plane = d.destCr
	}
	vc1LoopFilter(plane, d.cBlockBase(mbx, mby), d.cBufStride, 1, 8, d.pq)
}

func (d *intraDecoder) iVLoopFilter(mbx, mby int, flags int, blockNum int) {
	if blockNum&5 == 1 {
		return
	}
	if flags&edgeTop != 0 && blockNum&2 != 0 {
		return
	}
	if blockNum <= 3 {
		base := d.yBlockBase(mbx, mby, blockNum) + 4*d.yBufStride
		vc1LoopFilter(d.destY, base, 1, d.yBufStride, 16, d.pq)
		return
	}
	plane := d.destCb
	if blockNum != 4 {
		plane = d.destCr
	}
	base := d.cBlockBase(mbx, mby) + 4*d.cBufStride
	vc1LoopFilter(plane, base, 1, d.cBufStride, 8, d.pq)
}

func vc1LoopFilter(plane []byte, base, step, stride, length, pq int) {
	for i := 0; i < length; i += 4 {
		col := base + i*step
		if vc1FilterLine(plane, col, step, stride, pq) {
			vc1FilterLine(plane, col, 0, stride, pq)
			vc1FilterLine(plane, col+step, step, stride, pq)
			vc1FilterLine(plane, col+2*step, step, stride, pq)
		}
	}
}

func vc1FilterLine(plane []byte, base, step, stride, pq int) bool {
	at := func(row, col int) (int, bool) {
		p := base + row*stride + col*step
		if p < 0 || p >= len(plane) {
			return 0, false
		}
		return int(plane[p]), true
	}
	v0, ok := at(0, 0)
	if !ok {
		return false
	}
	v1, ok := at(-1, 0)
	if !ok {
		return false
	}
	v2, ok := at(1, 0)
	if !ok {
		return false
	}
	vm2, ok := at(-2, 0)
	if !ok {
		return false
	}
	a0 := (2*(vm2-v2) - 5*(v1-v0) + 4) >> 3
	sign := a0 >> 31
	if sign != 0 {
		a0 = -a0
	}
	if a0 >= pq {
		return false
	}
	vm4, ok1 := at(-4, 0)
	vm3, ok2 := at(-3, 0)
	v3, ok3 := at(3, 0)
	v4, ok4 := at(2, 0)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return false
	}
	a1 := absInt((2*(vm4-v1) - 5*(vm3-vm2) + 4) >> 3)
	a2 := absInt((2*(v0-v3) - 5*(v4-v2) + 4) >> 3)
	if a1 < a0 && a2 < a0 {
		clip := v1 - v0
		clipSign := clip >> 31
		if clipSign != 0 {
			clip = -clip
		}
		clip >>= 1
		if clip == 0 {
			return false
		}
		a3 := a1
		if a2 < a3 {
			a3 = a2
		}
		dv := (5 * (a3 - a0)) >> 3
		dSign := dv >> 31
		if dSign != 0 {
			dv = -dv
		}
		dSign ^= sign
		if (dv >> 31) != (clipSign >> 31) {
			return false
		}
		if dv > clip {
			dv = clip
		}
		if sign != 0 {
			dv = -dv
		}
		p0 := base
		p1 := base - stride
		if step != 1 {
			p1 = base - step
		}
		if p0 < 0 || p1 < 0 || p0 >= len(plane) || p1 >= len(plane) {
			return false
		}
		plane[p1] = clipU8(v1 - dv)
		plane[p0] = clipU8(v0 + dv)
		return true
	}
	return false
}
