//go:build goexperiment.simd && amd64 && !purego

package kernels

import (
	"simd/archsimd"

	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// simdRGBArea enables the AVX2 RGB row accumulator for the area downscaler.
const simdRGBArea = true

// rgbAccumRow420SIMD handles 8 pixels per step and returns how many it did
// (a multiple of 8); the caller finishes the tail. The LUT tables are exact
// linear functions of the sample (NewRGBLUT), so the slope and intercept read
// back from two entries reproduce every entry bit for bit, letting VPMULLD
// replace five gathers per pixel. Only AVX2 instructions are used.
func rgbAccumRow420SIMD(accR, accG, accB []int32, y, cb, cr []byte, lut *yuv.RGBLUT) int {
	n := len(accR) &^ 7
	if n == 0 {
		return 0
	}
	slope := func(t *[256]int32) (archsimd.Int32x8, archsimd.Int32x8) {
		return archsimd.BroadcastInt32x8(t[1] - t[0]), archsimd.BroadcastInt32x8(t[0])
	}
	ky, by := slope(&lut.Y)
	kcrr, bcrr := slope(&lut.CrR)
	kcbg, bcbg := slope(&lut.CbG)
	kcrg, bcrg := slope(&lut.CrG)
	kcbb, bcbb := slope(&lut.CbB)
	bg := bcbg.Add(bcrg)
	zero := archsimd.Int32x8{}
	max := archsimd.BroadcastInt32x8(255)
	accR, accG, accB = accR[:n], accG[:n], accB[:n]
	y, cb, cr = y[:n], cb[:n/2], cr[:n/2]
	for i := 0; i < n; i += 8 {
		yv := load8(y[i:]).Mul(ky).Add(by)
		c := dup4(cb[i/2:])
		r := dup4(cr[i/2:])
		rv := yv.Add(r.Mul(kcrr)).Add(bcrr)
		gv := yv.Add(c.Mul(kcbg)).Add(r.Mul(kcrg)).Add(bg)
		bv := yv.Add(c.Mul(kcbb)).Add(bcbb)
		archsimd.LoadInt32x8(accR[i:]).Add(rv.ShiftAllRight(8).Max(zero).Min(max)).Store(accR[i:])
		archsimd.LoadInt32x8(accG[i:]).Add(gv.ShiftAllRight(8).Max(zero).Min(max)).Store(accG[i:])
		archsimd.LoadInt32x8(accB[i:]).Add(bv.ShiftAllRight(8).Max(zero).Min(max)).Store(accB[i:])
	}
	archsimd.ClearAVXUpperBits()
	return n
}

// load8 zero-extends 8 bytes to int32 lanes (VPMOVZXBD).
func load8(p []byte) archsimd.Int32x8 {
	_ = p[7]
	v := uint64(p[0]) | uint64(p[1])<<8 | uint64(p[2])<<16 | uint64(p[3])<<24 |
		uint64(p[4])<<32 | uint64(p[5])<<40 | uint64(p[6])<<48 | uint64(p[7])<<56
	return archsimd.Uint64x2{}.SetElem(0, v).AsUint8x16().ExtendLo8ToUint32().AsInt32x8()
}

// dup4 expands 4 chroma bytes to 8 int32 lanes, each sample repeated twice.
func dup4(p []byte) archsimd.Int32x8 {
	_ = p[3]
	v := uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24
	c := archsimd.Uint32x4{}.SetElem(0, v).AsUint8x16().ExtendLo4ToUint32().AsInt32x4()
	return archsimd.Int32x8{}.SetLo(c.InterleaveLo(c)).SetHi(c.InterleaveHi(c))
}
