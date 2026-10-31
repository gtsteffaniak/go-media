package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

func (d *intraDecoder) decodeIBlocks() error {
	d.esc3LevelLen, d.esc3RunLen = 0, 0
	d.codingSet = codingSetY(d.pqIndex, d.yACTab)
	d.codingSet2 = codingSetC(d.pqIndex, d.cACTab)
	var blocks [6][64]int16
	for d.mbY = 0; d.mbY < d.mbh; d.mbY++ {
		d.firstSliceLn = d.mbY == 0
		for d.mbX = 0; d.mbX < d.endMBX; d.mbX++ {
			blocks = [6][64]int16{}
			if err := d.decodeMB(&blocks); err != nil {
				return err
			}
			d.storeMB(&blocks)
		}
	}
	return nil
}

func (d *intraDecoder) decodeMB(blocks *[6][64]int16) error {
	cbpVal, ok := d.r.vlcMBIntra()
	if !ok {
		return errs.Corrupt
	}
	cbp := int(cbpVal)
	acPred := d.r.bit()
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
		if err := d.decodeIBlock(k, val != 0, acPred, cs, &blocks[k]); err != nil {
			return err
		}
	}
	if d.r.left() < 0 {
		return errs.Corrupt
	}
	return nil
}
