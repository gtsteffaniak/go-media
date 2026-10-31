package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

type intraDecoder struct {
	seq                    seq
	w, h, mbw, mbh, endMBX int

	pq, halfPQ, pqIndex   int
	pquantizer            bool
	dcTable               int
	yACTab, cACTab        int
	codingSet, codingSet2 int

	leftBlkSh, topBlkSh int
	useTransposedZZ     bool

	overlap, loopFilter, rangeRedFrm bool
	dquantFrm                        bool
	condover                         int

	b8Stride              int
	mbStride              int
	lumaSize, chromaPlane int

	r bitReader

	dcVal      []int16
	acVal      []int16
	codedBlock []uint8

	mbX, mbY     int
	firstSliceLn bool

	esc3LevelLen, esc3RunLen int

	resY, resCb, resCr []int16 // reconstructed samples before clamping
	yStride, cStride   int
	pic                *Picture
}

func newIntraDecoder(config []byte, sample []byte, width, height int) (*intraDecoder, error) {
	s, err := parseSequence(config, width, height)
	if err != nil {
		return nil, err
	}
	if s.profile != 0 && s.profile != 1 {
		return nil, errUnsupportedProfile(s.profile)
	}
	if !s.resFastTX {
		// Without RES_FASTTX the reference decoder switches to a different
		// (WMV2-style) inverse transform that is not implemented here.
		return nil, errs.UnsupProfile("wmv3 keyframe", "WMV3 without fast transform")
	}
	d := &intraDecoder{
		seq: s, w: width, h: height,
		mbw: (width + 15) >> 4, mbh: (height + 15) >> 4,
		endMBX: (width + 15) >> 4, b8Stride: 2*((width+15)>>4) + 1, mbStride: ((width + 15) >> 4) + 1,
		r: newBitReader(sample), firstSliceLn: true,
	}
	d.useTransposedZZ = s.profile == 3 || s.resFastTX
	d.leftBlkSh, d.topBlkSh = blkShifts(s.resFastTX)
	d.lumaSize = (2*d.mbh + 1) * d.b8Stride
	d.chromaPlane = (d.mbh + 1) * d.mbStride
	nDC := d.lumaSize + 2*d.chromaPlane + 8
	d.dcVal = make([]int16, nDC)
	d.acVal = make([]int16, nDC*16)
	d.codedBlock = make([]uint8, nDC)
	ys, cs := d.mbw*16, d.mbw*8
	d.yStride, d.cStride = ys, cs
	d.resY = make([]int16, ys*d.mbh*16)
	d.resCb = make([]int16, cs*d.mbh*8)
	d.resCr = make([]int16, cs*d.mbh*8)
	d.pic = &Picture{
		Y: make([]byte, ys*d.mbh*16), Cb: make([]byte, cs*d.mbh*8), Cr: make([]byte, cs*d.mbh*8),
		YStride: ys, CStride: cs, Width: width, Height: height, FullRange: s.ranged,
	}
	return d, nil
}

// blockIndex mirrors FFmpeg's block_index layout: luma blocks on a b8 grid
// with a one-entry top/left border, followed by Cb and Cr planes on an MB
// grid with the same border. Border entries are never written, so neighbour
// reads at picture edges see zero.
func (d *intraDecoder) blockIndex(n int) int {
	switch {
	case n < 4:
		return (1+2*d.mbY+(n>>1))*d.b8Stride + 1 + 2*d.mbX + (n & 1)
	case n == 4:
		return d.lumaSize + (1+d.mbY)*d.mbStride + 1 + d.mbX
	default:
		return d.lumaSize + d.chromaPlane + (1+d.mbY)*d.mbStride + 1 + d.mbX
	}
}

// blockWrap is the distance to the block directly above block n.
func (d *intraDecoder) blockWrap(n int) int {
	if n < 4 {
		return d.b8Stride
	}
	return d.mbStride
}

func (d *intraDecoder) yDCScale() int { return int(wmv3DCScale[d.pq]) }
func (d *intraDecoder) cDCScale() int { return int(wmv3DCScale[d.pq]) }
