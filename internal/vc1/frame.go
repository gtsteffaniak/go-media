package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

func (d *intraDecoder) parseFrameHeader() error {
	if d.seq.profile == 3 {
		return d.parseFrameHeaderAdv()
	}
	return d.parseFrameHeaderSimple()
}

func (d *intraDecoder) parseFrameHeaderSimple() error {
	r := &d.r
	if d.seq.finterp {
		_ = r.bit()
	}
	r.skip(2)
	d.rangeRedFrm = false
	if d.seq.ranged {
		d.rangeRedFrm = r.bit()
	}
	if r.bit() {
		return errs.Unsup("wmv3 keyframe", "P-frame in keyframe sample")
	}
	if d.seq.maxB > 0 && !r.bit() {
		return errs.Unsup("wmv3 keyframe", "B-frame in keyframe sample")
	}
	r.skip(7)
	pqIndex := int(r.bits(5))
	if pqIndex == 0 {
		return errs.Corrupt
	}
	row := 0
	if d.seq.quantMode == 1 {
		row = 1
	}
	d.pqIndex = pqIndex
	d.pq = int(pquantTable[row][pqIndex])
	if pqIndex < 9 && r.bit() {
		d.halfPQ = 1
	}
	switch d.seq.quantMode {
	case 0:
		d.pquantizer = pqIndex < 9
	case 1:
		d.pquantizer = false
	case 2:
		d.pquantizer = r.bit()
	default:
		d.pquantizer = true
	}
	if d.seq.extendedMV {
		n := 0
		for n < 3 && !d.r.bit() {
			n++
		}
		_ = n
	}
	if d.seq.multires {
		r.skip(2)
	}
	if d.seq.resX8 {
		r.skip(1)
	}
	d.cACTab = decode012(r)
	d.yACTab = decode012(r)
	d.dcTable = 0
	if r.bit() {
		d.dcTable = 1
	}
	d.codingSet = codingSetY(pqIndex, d.yACTab)
	d.codingSet2 = codingSetC(pqIndex, d.cACTab)
	d.loopFilter = d.seq.loopFilter
	d.overlap = d.seq.overlap
	return nil
}

func (d *intraDecoder) parseFrameHeaderAdv() error {
	r := &d.r
	if d.seq.interlace && decode012(r) != 0 {
		return errs.UnsupProfile("wvc1 keyframe", "interlaced advanced sequence")
	}
	pt := 0
	for pt < 4 && !r.bit() {
		pt++
	}
	if !r.bit() || pt != 2 {
		return errs.Unsup("wvc1 keyframe", "non-I advanced frame")
	}
	if d.seq.tfcntr {
		r.skip(8)
	}
	if d.seq.broadcast {
		r.skip(2)
	}
	_ = r.bit()
	pqIndex := int(r.bits(5))
	if pqIndex == 0 {
		return errs.Corrupt
	}
	d.pqIndex = pqIndex
	d.pq = int(pquantTable[1][pqIndex])
	if pqIndex < 9 && r.bit() {
		d.halfPQ = 1
	}
	d.pquantizer = r.bit()
	if d.seq.quantMode == 3 {
		d.dquantFrm = r.bit()
	}
	if d.seq.postproc {
		r.skip(2)
	}
	if bitplaneDecode(r, d.mbw, d.mbh, d.mbw) < 0 {
		return errs.Corrupt
	}
	if d.seq.overlap && d.pq <= 8 {
		d.condover = decode012(r)
		if d.condover == 2 && bitplaneDecode(r, d.mbw, d.mbh, d.mbw) < 0 {
			return errs.Corrupt
		}
	}
	d.cACTab = decode012(r)
	d.yACTab = decode012(r)
	d.dcTable = 0
	if r.bit() {
		d.dcTable = 1
	}
	d.codingSet = codingSetY(pqIndex, d.yACTab)
	d.codingSet2 = codingSetC(pqIndex, d.cACTab)
	d.loopFilter = d.seq.loopFilter
	d.overlap = d.seq.overlap
	return nil
}
