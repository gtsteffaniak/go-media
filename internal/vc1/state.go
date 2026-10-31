package vc1

type intraDecoder struct {
	seq seq
	w, h, mbw, mbh, endMBX int

	pq, halfPQ, pqIndex int
	pquantizer          bool
	dcTable             int
	yACTab, cACTab      int
	codingSet, codingSet2 int

	leftBlkSh, topBlkSh int
	useTransposedZZ     bool

	overlap, loopFilter, rangeRedFrm bool
	dquantFrm                        bool
	condover                         int

	b8Stride int

	r bitReader

	dcVal      []int16
	acVal      []int16
	codedBlock []uint8

	mbX, mbY     int
	firstSliceLn bool

	esc3LevelLen, esc3RunLen int

	blkRing            [][][64]int16
	blkCur, blkLeft    int
	blkTop, blkTopLeft int
	blkSlots           int

	destY, destCb, destCr []byte
	yStride, cStride      int
	yBufStride, cBufStride int
	xPad, yPad            int
	pic                   *Picture
}

func newIntraDecoder(config []byte, sample []byte, width, height int) (*intraDecoder, error) {
	s, err := parseSequence(config, width, height)
	if err != nil {
		return nil, err
	}
	if s.profile != 0 && s.profile != 1 && s.profile != 3 {
		return nil, errUnsupportedProfile(s.profile)
	}
	d := &intraDecoder{
		seq: s, w: width, h: height,
		mbw: (width + 15) >> 4, mbh: (height + 15) >> 4,
		endMBX: (width + 15) >> 4, b8Stride: 2 * ((width + 15) >> 4),
		r: newBitReader(sample), firstSliceLn: true,
	}
	d.useTransposedZZ = s.profile == 3 || s.resFastTX
	d.leftBlkSh, d.topBlkSh = blkShifts(s.resFastTX)
	nDC := (2*d.mbh+2)*d.b8Stride + 8
	d.dcVal = make([]int16, nDC)
	d.acVal = make([]int16, nDC*16)
	d.codedBlock = make([]uint8, nDC)
	d.blkSlots = d.endMBX + 2
	d.blkRing = make([][][64]int16, d.blkSlots)
	for i := range d.blkRing {
		d.blkRing[i] = make([][64]int16, 6)
	}
	ys, cs := d.mbw*16, d.mbw*8
	d.yStride, d.cStride = ys, cs
	d.xPad, d.yPad = 4, 4
	d.yBufStride = ys + 2*d.xPad
	d.cBufStride = cs + 2*d.xPad
	yRows := d.mbh*16 + 2*d.yPad
	cRows := d.mbh*8 + 2*d.yPad
	d.destY = make([]byte, d.yBufStride*yRows)
	d.destCb = make([]byte, d.cBufStride*cRows)
	d.destCr = make([]byte, d.cBufStride*cRows)
	for i := range d.destY {
		d.destY[i] = 128
	}
	for i := range d.destCb {
		d.destCb[i] = 128
	}
	for i := range d.destCr {
		d.destCr[i] = 128
	}
	d.pic = &Picture{
		Y: make([]byte, ys*d.mbh*16), Cb: make([]byte, cs*d.mbh*8), Cr: make([]byte, cs*d.mbh*8),
		YStride: ys, CStride: cs, Width: width, Height: height, FullRange: s.ranged,
	}
	return d, nil
}

func (d *intraDecoder) copyPictureFromBuf() {
	for y := 0; y < d.mbh*16; y++ {
		src := d.destY[(d.yPad+y)*d.yBufStride+d.xPad:]
		copy(d.pic.Y[y*d.yStride:(y+1)*d.yStride], src[:d.yStride])
	}
	for y := 0; y < d.mbh*8; y++ {
		srcCb := d.destCb[(d.yPad+y)*d.cBufStride+d.xPad:]
		srcCr := d.destCr[(d.yPad+y)*d.cBufStride+d.xPad:]
		copy(d.pic.Cb[y*d.cStride:(y+1)*d.cStride], srcCb[:d.cStride])
		copy(d.pic.Cr[y*d.cStride:(y+1)*d.cStride], srcCr[:d.cStride])
	}
}

func (d *intraDecoder) blockIndex(n int) int {
	base := d.b8Stride*(2*d.mbY) + 2*d.mbX
	switch n {
	case 0:
		return base
	case 1:
		return base + 1
	case 2:
		return base + d.b8Stride
	case 3:
		return base + d.b8Stride + 1
	case 4:
		return base + 2 * d.b8Stride
	default:
		return base + 2*d.b8Stride + 1
	}
}

func (d *intraDecoder) yDCScale() int { return int(wmv3DCScale[d.pq]) }
func (d *intraDecoder) cDCScale() int { return int(wmv3DCScale[d.pq]) }

func (d *intraDecoder) initBlockRing() {
	d.blkCur, d.blkTop, d.blkTopLeft = 0, 1, 0
	d.blkLeft = d.blkSlots - 1
}

func (d *intraDecoder) advanceBlockRing() {
	d.blkTopLeft, d.blkTop = d.blkTop, d.blkCur
	d.blkLeft = (d.blkLeft + 1) % d.blkSlots
	d.blkCur = (d.blkCur + 1) % d.blkSlots
	for i := range d.blkRing[d.blkCur] {
		clear(d.blkRing[d.blkCur][i][:])
	}
}

func (d *intraDecoder) decodeIBlocksAdvanced() error { return d.decodeIBlocks() }
