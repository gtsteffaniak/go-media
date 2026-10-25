package decode

import (
	"errors"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

// Reconstruction of I-slice macroblocks (CAVLC). sliceDecoder keeps the
// neighbor context: the number of nonzero coefficients per 4x4 block (for nC)
// and which macroblocks have already been decoded (for neighbor availability).
// Supports one slice per frame (first_mb=0), frame MBs (no fields), 4:2:0/mono.

// Scan of luma 4x4 blocks within an MB → offset (in units of 4 pixels).
var luma4x4BlockX = [16]int{0, 1, 0, 1, 2, 3, 2, 3, 0, 1, 0, 1, 2, 3, 2, 3}
var luma4x4BlockY = [16]int{0, 0, 1, 1, 0, 0, 1, 1, 2, 2, 3, 3, 2, 2, 3, 3}

// qpcMap — mapping qPi→QPc for qPi in the range 30..51.
var qpcMap = [22]int{29, 30, 31, 32, 32, 33, 34, 34, 35, 35, 36, 36, 37, 37, 37, 38, 38, 38, 39, 39, 39, 39}

func chromaQP(qpY, offset int) int {
	qpi := qpY + offset
	if qpi < 0 {
		qpi = 0
	}
	if qpi > 51 {
		qpi = 51
	}
	if qpi < 30 {
		return qpi
	}
	return qpcMap[qpi-30]
}

type sliceDecoder struct {
	r     *bitstream.Reader
	h     *SliceHeader
	sps   *SPS
	pps   *PPS
	frame *Frame

	qp int // current QP_Y (accumulated via mb_qp_delta)

	w4, h4     int     // grid of luma 4x4 blocks over the whole frame
	cw4, ch4   int     // grid of chroma 4x4 blocks
	cbpw       int     // width of one chroma component in 4x4 blocks per MB (2 for 4:2:0)
	nzLuma     []uint8 // nonzero coefficients per luma 4x4 block
	nzCb, nzCr []uint8
	mbDecoded  []bool
	recon4     []bool // whether the luma 4x4 block is reconstructed (for intra 4x4)
	i4mode     []int8 // Intra4x4PredMode per 4x4 block (-1 = not 4x4)
	mbQP       []int  // QP_Y of each MB (for deblocking; I_PCM → 0)

	cb                 *cabacEngine
	cabacScratch       cabacEngine

	// Reused MB / residual scratch (avoid per-MB heap traffic).
	predLuma     [256]byte
	predChroma   [128]byte
	nTop16       [16]byte
	nLeft16      [16]byte
	nTop4        [8]byte
	nLeft4       [4]byte
	nTop8        [16]byte
	nLeft8       [8]byte
	levelScratch [16]int32
	coeffScratch [16]int32
	runsScratch  [16]int
	resCoeff     [64]int32
	resSig       [64]bool

	mbI4x4             []bool // MB is I_NxN (for mb_type context)
	mbIsPCM            []bool // MB is I_PCM
	mbCbp              []int  // per-MB coded_block_pattern (low 4 luma, high chroma)
	mbChromaPredMode   []int8 // per-MB intra_chroma_pred_mode (for context)
	cbfLumaDC          []bool // per-MB luma DC coded_block_flag
	cbfCbDC, cbfCrDC   []bool // per-MB chroma DC coded_block_flag (U/V)
	lastQpDeltaNonZero bool   // previous mb_qp_delta != 0 (delta_qp context)
	mbTransform8x8     []bool // per-MB transform_size_8x8_flag
}

// newSliceDecoder is used by unit tests; production decode uses acquireSliceDecoder.
func newSliceDecoder(r *bitstream.Reader, h *SliceHeader, sps *SPS, pps *PPS, frame *Frame) *sliceDecoder {
	return acquireSliceDecoder(r, h, sps, pps, frame)
}

// run iterates over the macroblocks of the slice (CAVLC I: without mb_skip_run).
func (sd *sliceDecoder) run() error {
	if sd.pps.EntropyCodingMode {
		return sd.runCabac()
	}
	addr := int(sd.h.FirstMB)
	total := sd.frame.MbWidth * sd.frame.MbHeight
	for {
		if addr >= total {
			return errors.New("h264: macroblock address went out of frame bounds")
		}
		mbType := sd.r.UE()
		mb, err := decodeIMBType(mbType)
		if err != nil {
			return err
		}
		mbX, mbY := addr%sd.frame.MbWidth, addr/sd.frame.MbWidth
		switch mb.Kind {
		case MbIPCM:
			sd.decodeIPCM(addr)
			sd.markMBRecon4(mbX, mbY)
			sd.mbQP[addr] = 0 // I_PCM: QP_Y=0 for deblocking
		case MbI16x16:
			if err := sd.decodeMB16(addr, mb); err != nil {
				return err
			}
			sd.markMBRecon4(mbX, mbY)
			sd.mbQP[addr] = sd.qp
		case MbINxN:
			if err := sd.decodeMBNxN(addr); err != nil {
				return err
			}
			sd.mbQP[addr] = sd.qp
		}
		if sd.r.Err() != nil {
			return sd.r.Err()
		}
		sd.mbDecoded[addr] = true
		addr++
		if !sd.r.MoreRBSPData() {
			break
		}
	}
	return nil
}

// combineNC combines neighbor counts into nC.
func combineNC(nA, nB int, leftAvail, topAvail bool) int {
	switch {
	case leftAvail && topAvail:
		return (nA + nB + 1) >> 1
	case leftAvail:
		return nA
	case topAvail:
		return nB
	default:
		return 0
	}
}

// lumaNC computes nC for a luma 4x4 block from the left/top neighbors.
func (sd *sliceDecoder) lumaNC(mbX, mbY, blkIdx int) int {
	bx, by := luma4x4BlockX[blkIdx], luma4x4BlockY[blkIdx]
	gx, gy := mbX*4+bx, mbY*4+by
	var nA, nB int
	leftAvail := bx > 0 || (mbX > 0 && sd.mbDecoded[mbY*sd.frame.MbWidth+mbX-1])
	if leftAvail {
		nA = int(sd.nzLuma[gy*sd.w4+gx-1])
	}
	topAvail := by > 0 || (mbY > 0 && sd.mbDecoded[(mbY-1)*sd.frame.MbWidth+mbX])
	if topAvail {
		nB = int(sd.nzLuma[(gy-1)*sd.w4+gx])
	}
	return combineNC(nA, nB, leftAvail, topAvail)
}

// chromaNC — the same for a chroma 4x4 block (nz is taken from the passed grid).
func (sd *sliceDecoder) chromaNC(nz []uint8, mbX, mbY, blkIdx int) int {
	bx, by := blkIdx%sd.cbpw, blkIdx/sd.cbpw
	gx, gy := mbX*sd.cbpw+bx, mbY*(sd.frame.MbHeightC/4)+by
	var nA, nB int
	leftAvail := bx > 0 || (mbX > 0 && sd.mbDecoded[mbY*sd.frame.MbWidth+mbX-1])
	if leftAvail {
		nA = int(nz[gy*sd.cw4+gx-1])
	}
	topAvail := by > 0 || (mbY > 0 && sd.mbDecoded[(mbY-1)*sd.frame.MbWidth+mbX])
	if topAvail {
		nB = int(nz[(gy-1)*sd.cw4+gx])
	}
	return combineNC(nA, nB, leftAvail, topAvail)
}

// decodeIPCM reads an I_PCM macroblock (raw samples) into the frame planes.
func (sd *sliceDecoder) decodeIPCM(addr int) {
	r, frame, sps := sd.r, sd.frame, sd.sps
	r.AlignToByte()
	mbX, mbY := addr%frame.MbWidth, addr/frame.MbWidth
	for i := 0; i < 256; i++ {
		frame.Y[(mbY*16+i/16)*frame.StrideY+mbX*16+i%16] = byte(r.U(int(sps.BitDepthLuma)))
	}
	if frame.ChromaArrayType == 0 {
		return
	}
	n := frame.MbWidthC * frame.MbHeightC
	for _, plane := range [][]byte{frame.Cb, frame.Cr} {
		for i := 0; i < n; i++ {
			x := mbX*frame.MbWidthC + i%frame.MbWidthC
			y := mbY*frame.MbHeightC + i/frame.MbWidthC
			plane[y*frame.StrideC+x] = byte(r.U(int(sps.BitDepthChroma)))
		}
	}
	// I_PCM resets the context: neighbor nC are treated as maximal (16).
	sd.setMBNonzero(mbX, mbY, 16)
}

// markMBRecon4 marks all luma 4x4 blocks of the MB as reconstructed
// (for I_16x16 / I_PCM, so that neighboring intra-4x4 blocks can predict from them).
func (sd *sliceDecoder) markMBRecon4(mbX, mbY int) {
	for by := 0; by < 4; by++ {
		for bx := 0; bx < 4; bx++ {
			sd.recon4[(mbY*4+by)*sd.w4+mbX*4+bx] = true
		}
	}
}

// setMBNonzero sets the given count on all 4x4 blocks of the MB (for I_PCM).
func (sd *sliceDecoder) setMBNonzero(mbX, mbY int, v uint8) {
	for by := 0; by < 4; by++ {
		for bx := 0; bx < 4; bx++ {
			sd.nzLuma[(mbY*4+by)*sd.w4+mbX*4+bx] = v
		}
	}
	if sd.frame.ChromaArrayType == 0 {
		return
	}
	chH := sd.frame.MbHeightC / 4
	for by := 0; by < chH; by++ {
		for bx := 0; bx < sd.cbpw; bx++ {
			sd.nzCb[(mbY*chH+by)*sd.cw4+mbX*sd.cbpw+bx] = v
			sd.nzCr[(mbY*chH+by)*sd.cw4+mbX*sd.cbpw+bx] = v
		}
	}
}
