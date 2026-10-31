package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

func (d *intraDecoder) decodeIBlocks() error {
	d.codingSet = codingSetY(d.pqIndex, d.yACTab)
	d.codingSet2 = codingSetC(d.pqIndex, d.cACTab)
	d.initBlockRing()
	for d.mbY = 0; d.mbY < d.mbh; d.mbY++ {
		d.mbX = 0
		d.firstSliceLn = d.mbY == 0
		for d.mbX = 0; d.mbX < d.endMBX; d.mbX++ {
			if err := d.decodeMB(); err != nil {
				return err
			}
			d.advanceBlockRing()
		}
	}
	return nil
}

func (d *intraDecoder) decodeMB() error {
	cbpVal, ok := d.r.vlcMBIntra()
	if !ok {
		return errs.Corrupt
	}
	cbp := int(cbpVal)
	acPred := d.r.bit()
	cur := d.blkRing[d.blkCur]
	for k := 0; k < 6; k++ {
		val := (cbp >> (5 - k)) & 1
		if k < 4 {
			pred, ptr := d.codedBlockPred(k)
			val ^= pred
			d.codedBlock[ptr] = uint8(val)
		}
		cs := d.codingSet
		if k >= 4 {
			cs = d.codingSet2
		}
		if err := d.decodeIBlock(k, val != 0, acPred, cs, &cur[blockMap[k]]); err != nil {
			return err
		}
	}
	return d.decodeMBTail(cur)
}

func (d *intraDecoder) decodeMBTail(cur [][64]int16) error {
	signed := false
	if d.overlap && d.pq >= 9 {
		d.iOverlapMB(cur)
		signed = true
		if d.rangeRedFrm {
			scaleBlocks(cur, 2, false)
		}
	} else if d.rangeRedFrm {
		scaleBlocks(cur, 2, true)
	}
	d.putBlocksDelayed(signed)
	if d.loopFilter {
		d.iLoopFilter()
	}
	return nil
}

func scaleBlocks(cur [][64]int16, mul int, sub64 bool) {
	for k := 0; k < 6; k++ {
		b := &cur[blockMap[k]]
		for i := range b {
			if sub64 {
				b[i] = (b[i] - 64) * int16(mul)
			} else {
				b[i] *= int16(mul)
			}
		}
	}
}
