package vc1

// Reconstruction runs on whole planes after every macroblock is decoded.
// The VC-1 spec defines overlap smoothing as all vertical block edges of the
// frame followed by all horizontal edges, and the in-loop deblocking filter
// as all horizontal edges followed by all vertical edges (SMPTE 421M 8.5,
// 8.6). FFmpeg pipelines the same operations with a one-row delay; running
// them frame-wide is equivalent and far simpler.

// storeMB writes the six inverse-transformed blocks (natural order: Y0 Y1 Y2
// Y3 Cb Cr) of the current macroblock into the residual planes.
func (d *intraDecoder) storeMB(blocks *[6][64]int16) {
	for k := 0; k < 4; k++ {
		x0 := d.mbX*16 + (k&1)*8
		y0 := d.mbY*16 + (k>>1)*8
		for y := 0; y < 8; y++ {
			copy(d.resY[(y0+y)*d.yStride+x0:], blocks[k][y*8:y*8+8])
		}
	}
	x0, y0 := d.mbX*8, d.mbY*8
	for y := 0; y < 8; y++ {
		copy(d.resCb[(y0+y)*d.cStride+x0:], blocks[4][y*8:y*8+8])
		copy(d.resCr[(y0+y)*d.cStride+x0:], blocks[5][y*8:y*8+8])
	}
}

// finishPicture applies overlap smoothing, range reduction, clamping and the
// loop filter, producing the output picture.
func (d *intraDecoder) finishPicture() {
	signed := d.overlap && d.pq >= 9
	type plane struct {
		res    []int16
		out    []byte
		stride int
		w, h   int
	}
	planes := [3]plane{
		{d.resY, d.pic.Y, d.yStride, d.mbw * 16, d.mbh * 16},
		{d.resCb, d.pic.Cb, d.cStride, d.mbw * 8, d.mbh * 8},
		{d.resCr, d.pic.Cr, d.cStride, d.mbw * 8, d.mbh * 8},
	}
	for _, p := range planes {
		if signed {
			overlapPlane(p.res, p.stride, p.w, p.h)
		}
		bias := 0
		if signed {
			bias = 128
		}
		for i, v := range p.res[:p.stride*p.h] {
			x := int(v)
			if d.rangeRedFrm {
				if signed {
					x *= 2
				} else {
					x = (x - 64) * 2
				}
			}
			p.out[i] = clipU8(x + bias)
		}
		if d.loopFilter {
			loopFilterPlane(p.out, p.stride, p.w, p.h, d.pq)
		}
	}
}

// overlapPlane runs the VC-1 overlap smoothing filter on every internal 8x8
// block edge: vertical edges first, then horizontal edges.
func overlapPlane(p []int16, stride, w, h int) {
	for x := 8; x < w; x += 8 {
		for y := 0; y < h; y++ {
			rnd1 := 4
			if y&1 != 0 {
				rnd1 = 3
			}
			rnd2 := 7 - rnd1
			row := p[y*stride:]
			a, b, c, dd := int(row[x-2]), int(row[x-1]), int(row[x]), int(row[x+1])
			d1 := a - dd
			d2 := a - dd + b - c
			row[x-2] = int16((a*8 - d1 + rnd1) >> 3)
			row[x-1] = int16((b*8 - d2 + rnd2) >> 3)
			row[x] = int16((c*8 + d2 + rnd1) >> 3)
			row[x+1] = int16((dd*8 + d1 + rnd2) >> 3)
		}
	}
	for y := 8; y < h; y += 8 {
		for x := 0; x < w; x++ {
			rnd1 := 4
			if x&1 != 0 {
				rnd1 = 3
			}
			rnd2 := 7 - rnd1
			i := y*stride + x
			a, b, c, dd := int(p[i-2*stride]), int(p[i-stride]), int(p[i]), int(p[i+stride])
			d1 := a - dd
			d2 := a - dd + b - c
			p[i-2*stride] = int16((a*8 - d1 + rnd1) >> 3)
			p[i-stride] = int16((b*8 - d2 + rnd2) >> 3)
			p[i] = int16((c*8 + d2 + rnd1) >> 3)
			p[i+stride] = int16((dd*8 + d1 + rnd2) >> 3)
		}
	}
}
