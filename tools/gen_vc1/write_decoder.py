#!/usr/bin/env python3
"""Write internal/vc1 decoder sources (idempotent)."""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2] / "internal" / "vc1"

FILES = {}

FILES["intra_decoder.go"] = r'''package vc1

import (
	"github.com/gtsteffaniak/go-media/internal/errs"
)

type intraDecoder struct {
	seq              seq
	width, height    int
	mbw, mbh         int
	endMBX           int
	yStride, cStride int

	r bitReader

	pq, pqindex      int
	halfpq           bool
	pquantizer       bool
	rangeredfrm      bool
	yAcTab, cAcTab   int
	dcTab            int
	codingSet        int
	codingSet2       int
	yDCScaleVal      int
	cDCScaleVal      int
	dquantFrm        bool

	esc3LevelLen, esc3RunLen int

	lw, cw int

	dcLuma   []int16
	dcChroma []int16
	acLuma   []int16
	acChroma []int16
	coded    []byte

	Y, Cb, Cr []byte
}

func newIntraDecoder(config []byte, sample []byte, width, height int) (*intraDecoder, error) {
	if width <= 0 || height <= 0 {
		return nil, errs.Corrupt
	}
	s, err := parseSequence(config, width, height)
	if err != nil {
		return nil, err
	}
	if s.profile == 2 {
		return nil, errs.UnsupProfile("wmv3 keyframe", "VC-1 Complex Profile")
	}
	mbw := (width + 15) / 16
	mbh := (height + 15) / 16
	d := &intraDecoder{
		seq: s, width: width, height: height,
		mbw: mbw, mbh: mbh, endMBX: mbw,
		yStride: mbw * 16, cStride: mbw * 8,
		r:  newBitReader(sample),
		lw: 2*mbw + 1, cw: mbw + 1,
	}
	d.allocBuffers()
	return d, nil
}

func (d *intraDecoder) allocBuffers() {
	nl := d.lw * (2*d.mbh + 1)
	nc := d.cw * (d.mbh + 1)
	d.dcLuma = make([]int16, nl)
	d.dcChroma = make([]int16, nc*2)
	d.acLuma = make([]int16, nl*16)
	d.acChroma = make([]int16, nc*16*2)
	d.coded = make([]byte, d.mbw*d.mbh*4)
	d.Y = make([]byte, d.yStride*d.height)
	d.Cb = make([]byte, d.cStride*(d.height/2))
	d.Cr = make([]byte, d.cStride*(d.height/2))
}

func (d *intraDecoder) picture() *Picture {
	return &Picture{
		Y: d.Y, Cb: d.Cb, Cr: d.Cr,
		YStride: d.yStride, CStride: d.cStride,
		Width: d.width, Height: d.height,
	}
}

func (d *intraDecoder) lumaDCIndex(mbx, mby, n int) int {
	bx := 2*mbx + (n & 1)
	by := 2*mby + (n >> 1)
	return by*d.lw + bx
}

func (d *intraDecoder) chromaDCIndex(mbx, mby, n int) int {
	off := (n - 4) * d.cw * d.mbh
	return off + mby*d.cw + mbx
}

func (d *intraDecoder) acLumaPtr(idx int) []int16 {
	return d.acLuma[idx*16 : idx*16+16]
}

func (d *intraDecoder) acChromaPtr(idx int) []int16 {
	return d.acChroma[idx*16 : idx*16+16]
}

func (d *intraDecoder) setCodingSets() {
	d.codingSet = codingSetY(d.pqindex, d.yAcTab)
	d.codingSet2 = codingSetC(d.pqindex, d.cAcTab)
	d.yDCScaleVal = int(wmv3DCScale[d.pq])
	d.cDCScaleVal = d.yDCScaleVal
}
'''

# Due to size limits, remaining files written in append below in main()

def main() -> None:
    ROOT.mkdir(parents=True, exist_ok=True)
    for name, body in FILES.items():
        p = ROOT / name
        p.write_text(body)
        print("wrote", p, len(body))


if __name__ == "__main__":
    main()
