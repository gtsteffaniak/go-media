package vc1

func (d *intraDecoder) iPredDC(n int) (pred int, dir int, dcPtr int) {
	scale := d.yDCScale()
	if n >= 4 {
		scale = d.cDCScale()
	}
	wrap := 2 * d.b8Stride
	if n >= 4 {
		wrap = d.b8Stride
	}
	idx := d.blockIndex(n)
	a, b, c := 0, 0, 0
	if idx >= wrap {
		a = int(d.dcVal[idx-wrap])
	}
	if idx > 0 {
		c = int(d.dcVal[idx-1])
	}
	if idx > wrap {
		b = int(d.dcVal[idx-1-wrap])
	}
	if d.pq < 9 || !d.seq.overlap {
		if d.firstSliceLn && n != 2 && n != 3 {
			b, a = int(dcPred[scale]), int(dcPred[scale])
		}
		if d.mbX == 0 && n != 1 && n != 3 {
			b, c = int(dcPred[scale]), int(dcPred[scale])
		}
	} else {
		if d.firstSliceLn && n != 2 && n != 3 {
			b, a = 0, 0
		}
		if d.mbX == 0 && n != 1 && n != 3 {
			b, c = 0, 0
		}
	}
	if absInt(a-b) <= absInt(b-c) {
		return c, 1, idx
	}
	return a, 0, idx
}

func (d *intraDecoder) codedBlockPred(n int) (pred int, ptr int) {
	idx := d.blockIndex(n)
	wrap := d.b8Stride
	a, b, c := 0, 0, 0
	if idx > 0 {
		a = int(d.codedBlock[idx-1])
	}
	if idx >= wrap {
		c = int(d.codedBlock[idx-wrap])
	}
	if idx > wrap {
		b = int(d.codedBlock[idx-1-wrap])
	}
	if b == c {
		pred = a
	} else {
		pred = c
	}
	return pred, idx
}
