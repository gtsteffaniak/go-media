package decode

import (
	"sync"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

var sliceDecPool sync.Pool

func acquireSliceDecoder(r *bitstream.Reader, h *SliceHeader, sps *SPS, pps *PPS, frame *Frame) *sliceDecoder {
	sd, _ := sliceDecPool.Get().(*sliceDecoder)
	if sd == nil {
		sd = &sliceDecoder{}
	}
	sd.prepare(r, h, sps, pps, frame)
	return sd
}

func releaseSliceDecoder(sd *sliceDecoder) {
	if sd == nil {
		return
	}
	sd.cb = nil
	sliceDecPool.Put(sd)
}

func (sd *sliceDecoder) prepare(r *bitstream.Reader, h *SliceHeader, sps *SPS, pps *PPS, frame *Frame) {
	sd.r = r
	sd.h = h
	sd.sps = sps
	sd.pps = pps
	sd.frame = frame
	sd.qp = int(h.SliceQP)
	sd.w4 = frame.MbWidth * 4
	sd.h4 = frame.MbHeight * 4

	mbCount := frame.MbWidth * frame.MbHeight
	wh4 := sd.w4 * sd.h4

	sd.mbDecoded = growBool(sd.mbDecoded, mbCount)
	sd.mbQP = growInt(sd.mbQP, mbCount)
	sd.nzLuma = growU8(sd.nzLuma, wh4)
	sd.recon4 = growBool(sd.recon4, wh4)
	sd.i4mode = growI8(sd.i4mode, wh4)
	for i := 0; i < wh4; i++ {
		sd.nzLuma[i] = 0
		sd.recon4[i] = false
		sd.i4mode[i] = -1
	}
	for i := 0; i < mbCount; i++ {
		sd.mbDecoded[i] = false
		sd.mbQP[i] = 0
	}

	if frame.ChromaArrayType != 0 {
		sd.cbpw = frame.MbWidthC / 4
		sd.cw4 = frame.MbWidth * sd.cbpw
		sd.ch4 = frame.MbHeight * (frame.MbHeightC / 4)
		csz := sd.cw4 * sd.ch4
		sd.nzCb = growU8(sd.nzCb, csz)
		sd.nzCr = growU8(sd.nzCr, csz)
		for i := 0; i < csz; i++ {
			sd.nzCb[i] = 0
			sd.nzCr[i] = 0
		}
	} else {
		sd.cbpw = 0
		sd.cw4 = 0
		sd.ch4 = 0
	}

	sd.mbTransform8x8 = growBool(sd.mbTransform8x8, mbCount)
	for i := 0; i < mbCount; i++ {
		sd.mbTransform8x8[i] = false
	}

	if pps.EntropyCodingMode {
		sd.mbI4x4 = growBool(sd.mbI4x4, mbCount)
		sd.mbIsPCM = growBool(sd.mbIsPCM, mbCount)
		sd.mbCbp = growInt(sd.mbCbp, mbCount)
		sd.mbChromaPredMode = growI8(sd.mbChromaPredMode, mbCount)
		sd.cbfLumaDC = growBool(sd.cbfLumaDC, mbCount)
		sd.cbfCbDC = growBool(sd.cbfCbDC, mbCount)
		sd.cbfCrDC = growBool(sd.cbfCrDC, mbCount)
		for i := 0; i < mbCount; i++ {
			sd.mbI4x4[i] = false
			sd.mbIsPCM[i] = false
			sd.mbCbp[i] = 0
			sd.mbChromaPredMode[i] = 0
			sd.cbfLumaDC[i] = false
			sd.cbfCbDC[i] = false
			sd.cbfCrDC[i] = false
		}
	} else {
		sd.mbI4x4 = nil
		sd.mbIsPCM = nil
		sd.mbCbp = nil
		sd.mbChromaPredMode = nil
		sd.cbfLumaDC = nil
		sd.cbfCbDC = nil
		sd.cbfCrDC = nil
	}
	sd.lastQpDeltaNonZero = false
}

func growBool(s []bool, n int) []bool {
	if len(s) < n {
		return make([]bool, n)
	}
	return s[:n]
}

func growInt(s []int, n int) []int {
	if len(s) < n {
		return make([]int, n)
	}
	return s[:n]
}

func growU8(s []uint8, n int) []uint8 {
	if len(s) < n {
		return make([]uint8, n)
	}
	return s[:n]
}

func growI8(s []int8, n int) []int8 {
	if len(s) < n {
		s = make([]int8, n)
	}
	return s[:n]
}
