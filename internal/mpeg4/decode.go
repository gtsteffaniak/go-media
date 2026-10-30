package mpeg4

import (
	"fmt"
	"sync"
)

// Picture is a decoded 4:2:0 I-VOP. The planes cover whole macroblocks;
// Width and Height are the VOL display size.
type Picture struct {
	Y, Cb, Cr        []byte
	YStride, CStride int
	Width, Height    int
	// FullRange is video_range from the visual object video_signal_type.
	FullRange bool
	// MatrixCoefficients is valid when ColorDescriptionValid is set
	// (ISO/IEC 23091-2 code points, as in H.264 VUI).
	MatrixCoefficients    uint8
	ColorDescriptionValid bool
	// Interlaced reports a VOL with interlaced=1 (field DCT allowed).
	Interlaced bool
	// QuantType is "h263" or "mpeg".
	QuantType string
	// Encoder names encoder user data seen ("DivX", "XviD"), if any.
	Encoder []string
}

// decoder is the per-picture state. It is pooled; reset sizes and clears
// every slice before use so nothing leaks between files.
type decoder struct {
	v          *vol
	h          vop
	mbw, mbh   int
	lw, cw     int
	dc         [3][]int16
	ac         [3][]int16
	qs         []int8
	qscale     int
	resX, resY int
	block      [64]int16
	pic        *Picture
}

var decoderPool = sync.Pool{New: func() any { return new(decoder) }}

func growInt16(s []int16, n int, fill int16) []int16 {
	if cap(s) < n {
		s = make([]int16, n)
	}
	s = s[:n]
	for i := range s {
		s[i] = fill
	}
	return s
}

func (d *decoder) reset(v *vol, h vop) {
	d.v, d.h = v, h
	d.mbw = (v.width + 15) / 16
	d.mbh = (v.height + 15) / 16
	d.lw = 2*d.mbw + 1
	d.cw = d.mbw + 1
	nl := (2*d.mbh + 1) * d.lw
	nc := (d.mbh + 1) * d.cw
	d.dc[0] = growInt16(d.dc[0], nl, 1024)
	d.dc[1] = growInt16(d.dc[1], nc, 1024)
	d.dc[2] = growInt16(d.dc[2], nc, 1024)
	d.ac[0] = growInt16(d.ac[0], nl*16, 0)
	d.ac[1] = growInt16(d.ac[1], nc*16, 0)
	d.ac[2] = growInt16(d.ac[2], nc*16, 0)
	n := d.mbw * d.mbh
	if cap(d.qs) < n {
		d.qs = make([]int8, n)
	}
	d.qs = d.qs[:n]
	clear(d.qs)
	d.qscale = h.quant
	d.resX, d.resY = 0, 0
	d.pic = nil
}

// release drops references to caller data before returning d to the pool.
func (d *decoder) release() {
	d.v = nil
	d.pic = nil
	decoderPool.Put(d)
}

// DecodeKeyframe decodes the first coded VOP of sample, which must be an
// I-VOP. config is the esds DecoderSpecificInfo (VOS/VO/VOL headers); it
// may be empty when the sample carries its own VOL. In-band headers
// replace those from config, as in ffmpeg.
//
// Errors name the exact reason: an errs.UnsupportedProfile error for a
// coding tool the decoder does not implement, errs.Corrupt for a damaged
// bitstream.
func DecodeKeyframe(config, sample []byte) (*Picture, error) {
	tablesOnce.Do(initTables)
	var v vol
	var err error
	scan := func(b []byte, inSample bool) (*Picture, bool) {
		var pic *Picture
		done := false
		sawVOP := false
		splitUnits(b, func(u unit) bool {
			switch {
			case u.code == scVOS:
				if len(u.payload) > 0 && u.payload[0] >= 0xE1 && u.payload[0] <= 0xE8 {
					err = unsup(fmt.Sprintf("Studio Profile (profile_and_level_indication 0x%02X) not supported", u.payload[0]))
					return false
				}
			case u.code == scVisualObj:
				v.parseVisualObject(u.payload)
			case u.code == scUserData:
				v.parseUserData(u.payload)
			case u.code >= scVOLFirst && u.code <= scVOLLast:
				if err = v.parseVOL(u.payload); err != nil {
					return false
				}
			case u.code == scVOP && inSample:
				sawVOP = true
				if !v.valid {
					err = corrupt("VOP before any VOL header (no DecoderSpecificInfo in esds and none in the sample)")
					return false
				}
				h, r, herr := v.parseVOPHeader(u.payload)
				if herr != nil {
					err = herr
					return false
				}
				if !h.coded {
					return true // N-VOP (DivX packed padding): try the next VOP
				}
				if h.codingType != 0 {
					err = unsup(fmt.Sprintf("sync sample starts with a %s-VOP, not an I-VOP (only intra VOPs are decoded)", vopTypeNames[h.codingType]))
					return false
				}
				pic, err = decodeIVOP(&v, h, r)
				done = true
				return false
			}
			return true
		})
		if inSample && !done && err == nil {
			if sawVOP {
				err = unsup("sync sample holds only not-coded VOPs (vop_coded=0)")
			} else if isShortHeader(b) {
				err = unsup("short video header (H.263 baseline in MPEG-4) not supported")
			} else {
				err = corrupt("sample has no VOP start code (00 00 01 B6)")
			}
		}
		return pic, done
	}
	scan(config, false)
	if err != nil {
		return nil, err
	}
	pic, _ := scan(sample, true)
	if err != nil {
		return nil, err
	}
	return pic, nil
}

// isShortHeader reports a short_video_start_marker (22 bits 0000 0000
// 0000 0000 1000 00) at the start of b.
func isShortHeader(b []byte) bool {
	return len(b) >= 3 && b[0] == 0 && b[1] == 0 && b[2]&0xFC == 0x80
}

func decodeIVOP(v *vol, h vop, r reader) (*Picture, error) {
	d := decoderPool.Get().(*decoder)
	defer d.release()
	d.reset(v, h)
	ys := d.mbw * 16
	cs := d.mbw * 8
	pic := &Picture{
		Y:  make([]byte, ys*d.mbh*16),
		Cb: make([]byte, cs*d.mbh*8),
		Cr: make([]byte, cs*d.mbh*8),

		YStride: ys, CStride: cs,
		Width: v.width, Height: v.height,
		FullRange: v.fullRange, MatrixCoefficients: v.matrix, ColorDescriptionValid: v.matrixValid,
		Interlaced: v.interlaced, QuantType: "h263",
		Encoder: v.userDataTags,
	}
	if v.mpegQuant {
		pic.QuantType = "mpeg"
	}
	d.pic = pic
	mbNum := d.mbw * d.mbh
	mbBits := 1
	for (1 << mbBits) < mbNum {
		mbBits++
	}
	for mb := 0; mb < mbNum; mb++ {
		if mb > 0 && v.resyncMarkers && isResync(&r) {
			if err := d.videoPacket(&r, mb, mbNum, mbBits); err != nil {
				return nil, err
			}
		}
		mbx, mby := mb%d.mbw, mb/d.mbw
		if err := d.decodeMB(&r, mbx, mby); err != nil {
			return nil, err
		}
		if r.over {
			return nil, corrupt(fmt.Sprintf("I-VOP data truncated at macroblock %d (x=%d y=%d) of %d", mb, mbx, mby, mbNum))
		}
	}
	return pic, nil
}

// isResync reports whether r is at byte-alignment stuffing (a 0 then 1s)
// followed by the 17-bit I-VOP resync_marker.
func isResync(r *reader) bool {
	n := 8 - r.pos&7
	if r.peek(n) != uint32(1)<<(n-1)-1 {
		return false
	}
	return r.peek(n+17)&0x1FFFF == 1
}

// videoPacket parses video_packet_header() (§6.2.5.2) for an I-VOP.
func (d *decoder) videoPacket(r *reader, mb, mbNum, mbBits int) error {
	r.skip(8 - r.pos&7)
	r.skip(17)
	n := int(r.bits(mbBits))
	if n != mb {
		if n < mb || n >= mbNum {
			return corrupt(fmt.Sprintf("video packet macroblock_number %d invalid at macroblock %d", n, mb))
		}
		return corrupt(fmt.Sprintf("video packet jumps from macroblock %d to %d (lost data)", mb, n))
	}
	if q := int(r.bits(d.v.quantPrecision)); q != 0 {
		d.qscale = q
	}
	if r.bit() { // header_extension_code
		for r.bit() {
			if r.over {
				return corrupt("video packet header truncated")
			}
		}
		r.marker()
		r.skip(d.v.timeIncBits)
		r.marker()
		r.skip(2 + 3) // vop_coding_type, intra_dc_vlc_thr (ignored, as in ffmpeg)
	}
	d.resX, d.resY = mb%d.mbw, mb/d.mbw
	d.cleanAC()
	return nil
}

// cleanAC zeroes the AC predictors a new video packet must not use, over
// the same span as ffmpeg's ff_mpeg4_clean_buffers.
func (d *decoder) cleanAC() {
	x, y := d.resX, d.resY
	l := (2*y)*d.lw + 2*x // block (2x-1, 2y-1) in the bordered grid
	clear(d.ac[0][l*16 : (l+2*d.lw+1)*16])
	c := y*d.cw + x
	clear(d.ac[1][c*16 : (c+d.cw+1)*16])
	clear(d.ac[2][c*16 : (c+d.cw+1)*16])
}

var dquantTab = [4]int{-1, -2, 1, 2}

func yDCScale(q int) int {
	switch {
	case q <= 4:
		return 8
	case q <= 8:
		return 2 * q
	case q <= 24:
		return q + 8
	}
	return 2*q - 16
}

func cDCScale(q int) int {
	switch {
	case q <= 4:
		return 8
	case q <= 24:
		return (q + 13) / 2
	}
	return q - 6
}

func (d *decoder) decodeMB(r *reader, mbx, mby int) error {
	var mcbpc int
	for {
		v, ok := r.readLUT(mcbpcLUT[:], 9)
		if !ok {
			return corrupt(fmt.Sprintf("invalid mcbpc VLC at macroblock x=%d y=%d", mbx, mby))
		}
		if v >= 0 {
			mcbpc = v
			break
		}
		if r.over {
			return corrupt("I-VOP data truncated in mcbpc stuffing")
		}
	}
	mbType, cbpc := mcbpc>>2, mcbpc&3
	acPred := r.bit()
	cbpy, ok := r.readLUT(cbpyLUT[:], 6)
	if !ok {
		return corrupt(fmt.Sprintf("invalid cbpy VLC at macroblock x=%d y=%d", mbx, mby))
	}
	cbp := cbpy<<2 | cbpc
	useDCVLC := d.qscale < d.h.dcThr
	if mbType == 4 {
		d.qscale += dquantTab[r.bits(2)]
		d.qscale = min(max(d.qscale, 1), 31)
	}
	fieldDCT := false
	if d.v.interlaced {
		fieldDCT = r.bit()
	}
	d.qs[mby*d.mbw+mbx] = int8(d.qscale)
	ys, cs := d.pic.YStride, d.pic.CStride
	for n := 0; n < 6; n++ {
		if err := d.decodeBlock(r, n, cbp>>(5-n)&1 != 0, useDCVLC, acPred, mbx, mby); err != nil {
			return err
		}
		switch {
		case n < 4 && fieldDCT:
			off := (mby*16+n>>1)*ys + mbx*16 + (n&1)*8
			idctPut(&d.block, d.pic.Y[off:], 2*ys)
		case n < 4:
			off := (mby*16+(n>>1)*8)*ys + mbx*16 + (n&1)*8
			idctPut(&d.block, d.pic.Y[off:], ys)
		case n == 4:
			idctPut(&d.block, d.pic.Cb[mby*8*cs+mbx*8:], cs)
		default:
			idctPut(&d.block, d.pic.Cr[mby*8*cs+mbx*8:], cs)
		}
	}
	return nil
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// roundedDiv is ffmpeg's ROUNDED_DIV.
func roundedDiv(a, b int) int {
	if a > 0 {
		return (a + b>>1) / b
	}
	return (a - b>>1) / b
}

// decodeBlock reads one intra block, applies DC/AC prediction and leaves
// dequantised coefficients in d.block.
func (d *decoder) decodeBlock(r *reader, n int, coded, useDCVLC, acPred bool, mbx, mby int) error {
	blk := &d.block
	*blk = [64]int16{}
	comp, wrap, idx, scale := 0, d.lw, 0, 0
	if n < 4 {
		bx, by := 2*mbx+n&1, 2*mby+n>>1
		idx = (by+1)*d.lw + bx + 1
		scale = yDCScale(d.qscale)
	} else {
		comp, wrap = n-3, d.cw
		idx = (mby+1)*d.cw + mbx + 1
		scale = cDCScale(d.qscale)
	}
	dc := d.dc[comp]
	a, b, c := int(dc[idx-1]), int(dc[idx-1-wrap]), int(dc[idx-wrap])
	// ffmpeg's packet-edge handling (ff_mpeg4_pred_dc): first_slice_line
	// holds from the packet start until macroblock (resX, resY+1), so the
	// row above belongs to an earlier packet and is treated as absent.
	firstLine := mby == d.resY || (mby == d.resY+1 && mbx < d.resX)
	if firstLine && n != 3 {
		if n != 2 {
			b, c = 1024, 1024
		}
		if n != 1 && mbx == d.resX {
			b, a = 1024, 1024
		}
	}
	if mbx == d.resX && mby == d.resY+1 && (n == 0 || n == 4 || n == 5) {
		b = 1024
	}
	pred, dir := a, 0
	if abs(a-b) < abs(b-c) {
		pred, dir = c, 1
	}
	pred = (pred + scale>>1) / scale

	pos := 0
	diff := 0
	if useDCVLC {
		var size int
		var ok bool
		if n < 4 {
			size, ok = r.readLUT(dcLumaLUT[:], 11)
		} else {
			size, ok = r.readLUT(dcChromaLU[:], 12)
		}
		if !ok {
			return corrupt(fmt.Sprintf("invalid dct_dc_size VLC at macroblock x=%d y=%d block %d", mbx, mby, n))
		}
		if size > 0 {
			v := int(r.bits(size))
			if v>>(size-1) == 0 {
				v -= 1<<size - 1
			}
			diff = v
			if size > 8 {
				r.marker()
			}
		}
		pos = 1
	}
	if coded {
		scan := &scanZigzag
		switch {
		case acPred && dir == 0:
			scan = &scanAltV
		case acPred:
			scan = &scanAltH
		case d.h.altScan:
			scan = &scanAltV
		}
		for {
			e := tcoefLUT[r.peek(tcoefBits)]
			if e.len == 0 {
				return corrupt(fmt.Sprintf("invalid TCOEF VLC at macroblock x=%d y=%d block %d", mbx, mby, n))
			}
			r.skip(int(e.len))
			var last, run, level int
			if e.run == escapeRun {
				switch {
				case !r.bit(): // escape type 1: level offset
					e2 := tcoefLUT[r.peek(tcoefBits)]
					if e2.len == 0 || e2.run == escapeRun {
						return corrupt("invalid TCOEF after escape type 1")
					}
					r.skip(int(e2.len))
					last, run = int(e2.last), int(e2.run)
					level = int(e2.level) + int(maxLevel[last][run])
					if r.bit() {
						level = -level
					}
				case !r.bit(): // escape type 2: run offset
					e2 := tcoefLUT[r.peek(tcoefBits)]
					if e2.len == 0 || e2.run == escapeRun {
						return corrupt("invalid TCOEF after escape type 2")
					}
					r.skip(int(e2.len))
					last, level = int(e2.last), int(e2.level)
					run = int(e2.run) + int(maxRun[last][level]) + 1
					if r.bit() {
						level = -level
					}
				default: // escape type 3: fixed length
					last = int(r.bits(1))
					run = int(r.bits(6))
					if !r.marker() {
						return corrupt(fmt.Sprintf("first marker bit missing in TCOEF escape 3 at macroblock x=%d y=%d", mbx, mby))
					}
					level = int(int32(r.bits(12)<<20) >> 20)
					if !r.marker() {
						return corrupt(fmt.Sprintf("second marker bit missing in TCOEF escape 3 at macroblock x=%d y=%d", mbx, mby))
					}
					if level == 0 {
						return corrupt("TCOEF escape 3 with level 0")
					}
				}
			} else {
				last, run, level = int(e.last), int(e.run), int(e.level)
				if r.bit() {
					level = -level
				}
			}
			pos += run
			if pos > 63 {
				return corrupt(fmt.Sprintf("more than 64 coefficients in block %d at macroblock x=%d y=%d", n, mbx, mby))
			}
			blk[scan[pos]] = int16(level)
			pos++
			if last != 0 {
				break
			}
			if r.over {
				return corrupt("I-VOP data truncated inside a block")
			}
		}
	}
	if !useDCVLC {
		diff = int(blk[0])
	}
	qf := diff + pred
	stored := qf * scale
	if stored&^2047 != 0 {
		if stored < 0 {
			stored = 0
		} else {
			stored = 2047
		}
	}
	dc[idx] = int16(stored)

	ac := d.ac[comp]
	cur := ac[idx*16 : idx*16+16]
	if acPred {
		if dir == 0 {
			nb := ac[(idx-1)*16 : idx*16]
			qn := 0
			if mbx > 0 {
				qn = int(d.qs[mby*d.mbw+mbx-1])
			}
			if mbx == 0 || qn == d.qscale || n == 1 || n == 3 {
				for i := 1; i < 8; i++ {
					blk[i*8] += nb[i]
				}
			} else {
				for i := 1; i < 8; i++ {
					blk[i*8] += int16(roundedDiv(int(nb[i])*qn, d.qscale))
				}
			}
		} else {
			nb := ac[(idx-wrap)*16 : (idx-wrap)*16+16]
			qn := 0
			if mby > 0 {
				qn = int(d.qs[(mby-1)*d.mbw+mbx])
			}
			if mby == 0 || qn == d.qscale || n == 2 || n == 3 {
				for i := 1; i < 8; i++ {
					blk[i] += nb[8+i]
				}
			} else {
				for i := 1; i < 8; i++ {
					blk[i] += int16(roundedDiv(int(nb[8+i])*qn, d.qscale))
				}
			}
		}
	}
	for i := 1; i < 8; i++ {
		cur[i] = blk[i*8]
		cur[8+i] = blk[i]
	}

	// Dequantise (ffmpeg dct_unquantize_h263_intra / mpeg2_intra, non-bitexact).
	blk[0] = int16(qf * scale)
	q := d.qscale
	if d.v.mpegQuant {
		m := &d.v.intraMatrix
		for i := 1; i < 64; i++ {
			if l := int(blk[i]); l > 0 {
				blk[i] = int16((l * 2 * q * int(m[i])) >> 4)
			} else if l < 0 {
				blk[i] = int16(-((-l * 2 * q * int(m[i])) >> 4))
			}
		}
	} else {
		qmul, qadd := 2*q, (q-1)|1
		for i := 1; i < 64; i++ {
			if l := int(blk[i]); l > 0 {
				blk[i] = int16(l*qmul + qadd)
			} else if l < 0 {
				blk[i] = int16(l*qmul - qadd)
			}
		}
	}
	return nil
}
