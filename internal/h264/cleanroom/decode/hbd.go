package decode

import h264k "github.com/gtsteffaniak/go-media/internal/kernels/h264"

// High-bit-depth (9- and 10-bit 4:2:0 / monochrome) reconstruction.
//
// When the SPS signals bit_depth_luma/chroma above 8, the frame carries
// uint16 sample planes (Frame.Y16/Cb16/Cr16) and every reconstruction site
// in the macroblock decoders branches here once per block. Entropy decoding
// (CAVLC/CABAC) is shared with the 8-bit path; only prediction, dequant,
// residual add, I_PCM and deblocking differ:
//
//   - QPY runs from -QpBdOffsetY to 51 (QpBdOffsetY = 6*(BitDepthY-8)),
//     mb_qp_delta wraps modulo 52+QpBdOffsetY (spec 7.4.5) and dequant
//     uses QP'Y = QPY+QpBdOffsetY; chroma uses QP'C = QPC+QpBdOffsetC with
//     qPI clipped to [-QpBdOffsetC, 51] (8.5.8).
//   - Dequantized coefficients stay 32-bit (no int16 saturation), as in
//     ffmpeg's high-bit-depth h264 (dctcoef = int32).
//   - Samples clip to (1<<BitDepth)-1 and DC prediction without neighbors
//     is 1<<(BitDepth-1).
//   - Deblocking scales alpha/beta/tC0 by 1<<(BitDepth-8) (hbd_deblock.go).
//
// After the slice is reconstructed (and deblocked) the 16-bit planes are
// rounded to the 8-bit Y/Cb/Cr planes, (v + (1<<(bd-9))) >> (bd-8), so
// colour conversion and scaling downstream are unchanged. The 8-bit decode
// path (and its SIMD kernels) is not touched by any of this; each branch
// is a single predictable flag test.

// hbdState is the per-slice high-bit-depth configuration.
type hbdState struct {
	hbd      bool // frame has 16-bit planes
	qpOff    int  // QpBdOffsetY
	qpOffC   int  // QpBdOffsetC
	mxY, mxC int  // (1<<BitDepth)-1
	wTop16   [16]uint16
	wLeft16  [16]uint16
	wTop4    [8]uint16
	wLeft4   [4]uint16
	wTop8    [16]uint16
	wLeft8   [8]uint16
	wPred    [64]uint16
}

func (sd *sliceDecoder) prepareHBD(f *Frame) {
	sd.hbd = f.Y16 != nil
	if !sd.hbd {
		sd.qpOff, sd.qpOffC = 0, 0
		return
	}
	sd.qpOff = 6 * (f.BitDepthY - 8)
	sd.qpOffC = 6 * (f.BitDepthC - 8)
	sd.mxY = 1<<f.BitDepthY - 1
	sd.mxC = 1<<f.BitDepthC - 1
}

// addQPDelta applies mb_qp_delta to QPY (spec 7.4.5, equation 7-37).
func (sd *sliceDecoder) addQPDelta(delta int) {
	if sd.qpOff == 0 {
		sd.qp = (sd.qp + delta + 52) % 52
		return
	}
	n := 52 + sd.qpOff
	v := (sd.qp + delta + 52 + 2*sd.qpOff) % n
	if v < 0 {
		v += n
	}
	sd.qp = v - sd.qpOff
}

// dqQP returns QP'Y for the current macroblock (QPY + QpBdOffsetY).
func (sd *sliceDecoder) dqQP() int { return sd.qp + sd.qpOff }

// chromaDqQP returns QP'C for component comp from QP'Y.
func (sd *sliceDecoder) chromaDqQP(comp, qpYPrime int) int {
	if sd.qpOff == 0 && sd.qpOffC == 0 {
		return chromaQPFor(sd.pps, comp, qpYPrime)
	}
	return sd.chromaQPW(comp, qpYPrime-sd.qpOff) + sd.qpOffC
}

// chromaQPW maps QPY to QPC (not primed) for high bit depth: qPI is clipped
// to [-QpBdOffsetC, 51] and values below 30 map to themselves (Table 8-15).
func (sd *sliceDecoder) chromaQPW(comp, qpY int) int {
	off := sd.pps.ChromaQPIndexOffset
	if comp == 1 {
		off = sd.pps.SecondChromaQPIndexOffset
	}
	qpi := clip3(-sd.qpOffC, 51, qpY+int(off))
	if qpi < 30 {
		return qpi
	}
	return qpcMap[qpi-30]
}

// dq4 dequantizes a raster 4x4 block at QP' qp.
func (sd *sliceDecoder) dq4(coef [16]int32, qp int) [16]int32 {
	if sd.hbd {
		return dequant4x4Wide(coef, qp)
	}
	return dequant4x4(coef, qp)
}

// dqScan4 maps scan order to raster and dequantizes at QP' qp.
func (sd *sliceDecoder) dqScan4(scan []int32, qp int) [16]int32 {
	if sd.hbd {
		return dequant4x4Wide(inverseScan4x4(scan), qp)
	}
	return h264k.DequantScan4x4(scan, qp)
}

// dequant4x4Wide is dequant4x4 without the int16 saturation; it clamps only
// to the int32 range so hostile levels cannot wrap.
func dequant4x4Wide(coef [16]int32, qp int) [16]int32 {
	m, shift := qp%6, qp/6
	var d [16]int32
	for idx, c := range coef {
		if c == 0 {
			continue
		}
		v := int64(c) * int64(levelScale4x4(m, idx/4, idx%4))
		if shift >= 4 {
			v <<= uint(shift - 4)
		} else {
			v = (v + 1<<uint(3-shift)) >> uint(4-shift)
		}
		d[idx] = clampI32(v)
	}
	return d
}

func clampI32(v int64) int32 {
	const lim = 1 << 30 // headroom for the transform butterflies
	if v > lim {
		return lim
	}
	if v < -lim {
		return -lim
	}
	return int32(v)
}

// ---- prediction + residual add (16-bit planes) ----

// predLuma16W predicts an I_16x16 luma MB into Y16.
func (sd *sliceDecoder) predLuma16W(mbX, mbY, mode int) {
	f := sd.frame
	top, left := sd.wTop16[:], sd.wLeft16[:]
	availL := mbX > 0 && sd.mbDecoded[mbY*f.MbWidth+mbX-1]
	availT := mbY > 0 && sd.mbDecoded[(mbY-1)*f.MbWidth+mbX]
	x0, y0 := mbX*16, mbY*16
	var corner uint16
	if availT {
		copy(top, f.Y16[(y0-1)*f.StrideY+x0:][:16])
	}
	if availL {
		for y := 0; y < 16; y++ {
			left[y] = f.Y16[(y0+y)*f.StrideY+x0-1]
		}
	}
	if availL && availT {
		corner = f.Y16[(y0-1)*f.StrideY+x0-1]
	}
	predictIntra16x16AtW(sd.mxY, f.Y16, f.StrideY, x0, y0, mode, top, left, corner, availL, availT)
}

// addLuma16W adds the 16 residual blocks (raster order) of an I_16x16 MB.
func (sd *sliceDecoder) addLuma16W(mbX, mbY int, blocks *[16][16]int32, nz []uint8) {
	f := sd.frame
	for ri := 0; ri < 16; ri++ {
		if nz[ri] == 0 && blocks[ri][0] == 0 {
			continue
		}
		res := inverseTransform4x4(blocks[ri])
		addRes4W(f.Y16, f.StrideY, mbX*16+(ri%4)*4, mbY*16+(ri/4)*4, &res, sd.mxY)
	}
}

// addRes4W adds a 4x4 residual in place with clipping to [0, mx].
func addRes4W(plane []uint16, stride, x0, y0 int, res *[16]int32, mx int) {
	for y := 0; y < 4; y++ {
		row := plane[(y0+y)*stride+x0:][:4]
		for x := 0; x < 4; x++ {
			row[x] = clipW(mx, int(row[x])+int(res[y*4+x]))
		}
	}
}

// planeC returns the 16-bit chroma plane for comp (0 Cb, 1 Cr).
func (f *Frame) planeC(comp int) []uint16 {
	if comp == 0 {
		return f.Cb16
	}
	return f.Cr16
}

// predChromaW predicts one 4:2:0 chroma component of an MB into its plane.
func (sd *sliceDecoder) predChromaW(comp, mbX, mbY, mode int) {
	f := sd.frame
	buf := f.planeC(comp)
	w, hh := f.MbWidthC, f.MbHeightC
	top, left := sd.wTop16[:w], sd.wLeft16[:hh]
	availL := mbX > 0 && sd.mbDecoded[mbY*f.MbWidth+mbX-1]
	availT := mbY > 0 && sd.mbDecoded[(mbY-1)*f.MbWidth+mbX]
	x0, y0 := mbX*w, mbY*hh
	var corner uint16
	if availT {
		copy(top, buf[(y0-1)*f.StrideC+x0:][:w])
	}
	if availL {
		for y := 0; y < hh; y++ {
			left[y] = buf[(y0+y)*f.StrideC+x0-1]
		}
	}
	if availL && availT {
		corner = buf[(y0-1)*f.StrideC+x0-1]
	}
	predictIntraChroma8x8AtW(sd.mxC, buf, f.StrideC, x0, y0, mode, top, left, corner, availL, availT)
}

// addChromaW adds the four 4x4 residual blocks of one chroma component.
func (sd *sliceDecoder) addChromaW(comp, cx0, cy0 int, blocks *[4][16]int32, nz []uint8) {
	f := sd.frame
	buf := f.planeC(comp)
	for blk := 0; blk < 4; blk++ {
		if nz[blk] == 0 && blocks[blk][0] == 0 {
			continue
		}
		res := inverseTransform4x4(blocks[blk])
		addRes4W(buf, f.StrideC, cx0+(blk%2)*4, cy0+(blk/2)*4, &res, sd.mxC)
	}
}

// recon4x4W predicts luma 4x4 block (gx,gy) and adds the dequantized
// coefficients d.
func (sd *sliceDecoder) recon4x4W(gx, gy int, d [16]int32) {
	f := sd.frame
	top, left := sd.wTop4[:], sd.wLeft4[:]
	px0, py0 := gx*4, gy*4
	availTop := gy > 0 && sd.recon4[(gy-1)*sd.w4+gx]
	availLeft := gx > 0 && sd.recon4[gy*sd.w4+gx-1]
	availTL := gx > 0 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx-1]
	availTR := gx+1 < sd.w4 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx+1]
	var corner uint16
	if availTop {
		copy(top[:4], f.Y16[(py0-1)*f.StrideY+px0:][:4])
	}
	if availTR {
		copy(top[4:], f.Y16[(py0-1)*f.StrideY+px0+4:][:4])
	} else if availTop {
		for i := 4; i < 8; i++ {
			top[i] = top[3]
		}
	}
	if availLeft {
		for i := 0; i < 4; i++ {
			left[i] = f.Y16[(py0+i)*f.StrideY+px0-1]
		}
	}
	if availTL {
		corner = f.Y16[(py0-1)*f.StrideY+px0-1]
	}
	pred := sd.wPred[:16]
	predictIntra4x4W(sd.mxY, pred, int(sd.i4mode[gy*sd.w4+gx]), top, left, corner, availLeft, availTop)
	res := inverseTransform4x4(d)
	for y := 0; y < 4; y++ {
		row := f.Y16[(py0+y)*f.StrideY+px0:][:4]
		for x := 0; x < 4; x++ {
			row[x] = clipW(sd.mxY, int(pred[y*4+x])+int(res[y*4+x]))
		}
	}
}

// recon8x8W predicts luma 8x8 block (gx,gy) at pixel (px0,py0) and adds the
// inverse-transformed residual res.
func (sd *sliceDecoder) recon8x8W(gx, gy, px0, py0 int, res *[64]int32) {
	f := sd.frame
	top, left := sd.wTop8[:], sd.wLeft8[:]
	availTop := gy > 0 && sd.recon4[(gy-1)*sd.w4+gx]
	availLeft := gx > 0 && sd.recon4[gy*sd.w4+gx-1]
	availTL := gx > 0 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx-1]
	availTR := gx+2 < sd.w4 && gy > 0 && sd.recon4[(gy-1)*sd.w4+gx+2]
	var corner uint16
	if availTop {
		copy(top[:8], f.Y16[(py0-1)*f.StrideY+px0:][:8])
	}
	if availTR {
		copy(top[8:], f.Y16[(py0-1)*f.StrideY+px0+8:][:8])
	} else if availTop {
		for i := 8; i < 16; i++ {
			top[i] = top[7]
		}
	}
	if availLeft {
		for i := 0; i < 8; i++ {
			left[i] = f.Y16[(py0+i)*f.StrideY+px0-1]
		}
	}
	if availTL {
		corner = f.Y16[(py0-1)*f.StrideY+px0-1]
	}
	ft, fl, fc := filterRef8x8W(top, left, corner, availTop, availLeft, availTL)
	pred := sd.wPred[:64]
	predictIntra8x8W(sd.mxY, pred, int(sd.i4mode[gy*sd.w4+gx]), ft, fl, fc, availLeft, availTop)
	for y := 0; y < 8; y++ {
		row := f.Y16[(py0+y)*f.StrideY+px0:][:8]
		for x := 0; x < 8; x++ {
			row[x] = clipW(sd.mxY, int(pred[y*8+x])+int(res[y*8+x]))
		}
	}
}

// decodeIPCMW reads I_PCM samples at the stream bit depth into the 16-bit planes.
func (sd *sliceDecoder) decodeIPCMW(mbX, mbY int) {
	r, f := sd.r, sd.frame
	for i := 0; i < 256; i++ {
		f.Y16[(mbY*16+i/16)*f.StrideY+mbX*16+i%16] = uint16(r.U(f.BitDepthY))
	}
	if f.ChromaArrayType == 0 {
		return
	}
	n := f.MbWidthC * f.MbHeightC
	for _, plane := range [][]uint16{f.Cb16, f.Cr16} {
		for i := 0; i < n; i++ {
			x := mbX*f.MbWidthC + i%f.MbWidthC
			y := mbY*f.MbHeightC + i/f.MbWidthC
			plane[y*f.StrideC+x] = uint16(r.U(f.BitDepthC))
		}
	}
}

// downconvert rounds the 16-bit planes into the 8-bit Y/Cb/Cr planes.
func (f *Frame) downconvert() {
	down(f.Y, f.Y16, f.BitDepthY)
	if f.ChromaArrayType != 0 {
		down(f.Cb, f.Cb16, f.BitDepthC)
		down(f.Cr, f.Cr16, f.BitDepthC)
	}
}

func down(dst []byte, src []uint16, bd int) {
	if bd == 8 {
		for i, v := range src {
			dst[i] = byte(v)
		}
		return
	}
	sh := uint(bd - 8)
	rnd := uint16(1) << (sh - 1)
	mx := uint16(1)<<bd - 1
	for i, v := range src {
		if v > mx {
			v = mx
		}
		w := (v + rnd) >> sh
		if w > 255 {
			w = 255
		}
		dst[i] = byte(w)
	}
}
