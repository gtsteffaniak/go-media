package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

func (d *intraDecoder) decodeDC(n int) (int, error) {
	v, ok := d.r.vlcDC(n >= 4, d.dcTable)
	if !ok {
		return 0, errs.Corrupt
	}
	dcdiff := int(v)
	if dcdiff != 0 {
		m := 0
		if d.pq == 1 || d.pq == 2 {
			m = 3 - d.pq
		}
		if dcdiff == 119 {
			dcdiff = int(d.r.bits(8 + m))
		} else if m > 0 {
			dcdiff = (dcdiff << m) + int(d.r.bits(m)) - ((1 << m) - 1)
		}
		if d.r.bit() {
			dcdiff = -dcdiff
		}
	}
	pred, _, idx := d.iPredDC(n)
	dcdiff += pred
	d.dcVal[idx] = int16(dcdiff)
	scale := d.yDCScale()
	if n >= 4 {
		scale = d.cDCScale()
	}
	return dcdiff * scale, nil
}

func (d *intraDecoder) decodeIBlock(n int, coded, acPred bool, cs int, block *[64]int16) error {
	dc, err := d.decodeDC(n)
	if err != nil {
		return err
	}
	block[0] = int16(dc)
	idx := d.blockIndex(n)
	acBase := idx * 16
	acLeft, acTop := acBase, acBase
	wrap := 2 * d.b8Stride
	if n >= 4 {
		wrap = d.b8Stride
	}
	_, dir, _ := d.iPredDC(n)
	if dir == 1 && acLeft >= 16 {
		acLeft -= 16
	}
	if dir == 0 && acTop >= 16*wrap {
		acTop -= 16 * wrap
	}
	scale := d.pq*2 + d.halfPQ
	if coded {
		zz := acZZTables(d.seq.resFastTX, acPred, dir == 0)
		i := 1
		for {
			last, skip, val, ok := d.r.decodeAcCoeff(cs, &d.esc3LevelLen, &d.esc3RunLen, d.pq, d.dquantFrm)
			if !ok {
				return errs.Corrupt
			}
			i += skip
			if i > 63 {
				break
			}
			block[zz[i]] = int16(val)
			i++
			if last {
				break
			}
		}
		if acPred {
			sh := d.leftBlkSh
			ac := d.acVal[acLeft:]
			if dir == 0 {
				sh = d.topBlkSh
				ac = d.acVal[acTop+8:]
			}
			for k := 1; k < 8; k++ {
				block[k<<sh] += ac[k]
			}
		}
		acOut := d.acVal[acBase:]
		for k := 1; k < 8; k++ {
			acOut[k] = block[k<<d.leftBlkSh]
			acOut[k+8] = block[k<<d.topBlkSh]
		}
		for k := 1; k < 64; k++ {
			if block[k] != 0 {
				block[k] *= int16(scale)
				if !d.pquantizer {
					if block[k] < 0 {
						block[k] -= int16(d.pq)
					} else {
						block[k] += int16(d.pq)
					}
				}
			}
		}
	} else {
		acOut := d.acVal[acBase:]
		clear(acOut)
		if acPred {
			sh := d.leftBlkSh
			ac := d.acVal[acLeft:]
			if dir == 0 {
				sh = d.topBlkSh
				ac = d.acVal[acTop+8:]
				copy(acOut[8:], ac[:8])
			} else {
				copy(acOut[:8], ac[:8])
			}
			for k := 1; k < 8; k++ {
				block[k<<sh] = ac[k] * int16(scale)
				if !d.pquantizer && block[k<<sh] != 0 {
					if block[k<<sh] < 0 {
						block[k<<sh] -= int16(d.pq)
					} else {
						block[k<<sh] += int16(d.pq)
					}
				}
			}
		}
	}
	invTrans8x8(block)
	return nil
}
