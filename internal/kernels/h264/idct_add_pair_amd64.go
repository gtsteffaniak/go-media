//go:build goexperiment.simd && amd64 && !purego

package h264

import (
	"encoding/binary"
	"simd/archsimd"
)

// simdArchKernels reports that this build links real archsimd kernel bodies.
const simdArchKernels = true

// Every instruction used here is AVX or AVX2; KernelsUseSIMD gates on AVX2 only,
// so AVX-512-only archsimd methods (TruncTo*, Uint8x16.Permute, …) must not appear.
// TestSIMDKernelsAVX2Only enforces that on the compiled binary.
//
// Every exported-path kernel that touches 256/512-bit registers ends with
// archsimd.ClearAVXUpperBits (VZEROUPPER). Go's own code (memmove, the X15
// zero register, float math) uses legacy SSE encodings, and dirty upper YMM
// bits make every one of those instructions pay a false-dependency penalty
// until the next VZEROUPPER. Without it this kernel was 5× faster in isolation
// but made the whole decode ~12% slower. TestSIMDKernelsClearUpperBits checks it.

// packLowBytes picks byte 2k of each uint16 lane into the low 8 bytes (VPSHUFB).
var packLowBytes = archsimd.LoadInt8x16Array(&[16]int8{0, 2, 4, 6, 8, 10, 12, 14, -1, -1, -1, -1, -1, -1, -1, -1})

// idctAdd4x4PairSIMD runs the H.264 4×4 inverse transform on two horizontally
// adjacent blocks a (left) and b (right) and adds the residual in place to the
// 8×4 pixel area at plane[off:], clipping to 8 bits.
//
// Layout: one Int32x8 per pixel row, block a in the low 128-bit lane and block b
// in the high lane. AVX2 "Grouped" ops act per 128-bit lane, so the two blocks
// are transposed and transformed independently in the same registers, and the
// result row is exactly the 8 contiguous output pixels.
func idctAdd4x4PairSIMD(plane []byte, off, stride int, a, b *[16]int32) {
	r0 := join2(a, b, 0)
	r1 := join2(a, b, 4)
	r2 := join2(a, b, 8)
	r3 := join2(a, b, 12)

	// Horizontal (row) pass: transpose so each register holds one column, then
	// the butterfly runs lane-wise across registers.
	c0, c1, c2, c3 := transpose4x4x2(r0, r1, r2, r3)
	f0, f1, f2, f3 := butterfly4x8(c0, c1, c2, c3)
	// Vertical (column) pass on rows.
	g0, g1, g2, g3 := transpose4x4x2(f0, f1, f2, f3)
	h0, h1, h2, h3 := butterfly4x8(g0, g1, g2, g3)

	round := archsimd.BroadcastInt32x8(32)
	addRow8(plane, off, h0.Add(round).ShiftAllRight(6))
	addRow8(plane, off+stride, h1.Add(round).ShiftAllRight(6))
	addRow8(plane, off+2*stride, h2.Add(round).ShiftAllRight(6))
	addRow8(plane, off+3*stride, h3.Add(round).ShiftAllRight(6))
	archsimd.ClearAVXUpperBits()
}

func transpose4x4x2(r0, r1, r2, r3 archsimd.Int32x8) (archsimd.Int32x8, archsimd.Int32x8, archsimd.Int32x8, archsimd.Int32x8) {
	t0 := r0.InterleaveLoGrouped(r1).AsInt64x4() // a00 a10 a01 a11
	t1 := r0.InterleaveHiGrouped(r1).AsInt64x4() // a02 a12 a03 a13
	t2 := r2.InterleaveLoGrouped(r3).AsInt64x4() // a20 a30 a21 a31
	t3 := r2.InterleaveHiGrouped(r3).AsInt64x4() // a22 a32 a23 a33
	return t0.InterleaveLoGrouped(t2).AsInt32x8(),
		t0.InterleaveHiGrouped(t2).AsInt32x8(),
		t1.InterleaveLoGrouped(t3).AsInt32x8(),
		t1.InterleaveHiGrouped(t3).AsInt32x8()
}

func butterfly4x8(x0, x1, x2, x3 archsimd.Int32x8) (archsimd.Int32x8, archsimd.Int32x8, archsimd.Int32x8, archsimd.Int32x8) {
	e0 := x0.Add(x2)
	e1 := x0.Sub(x2)
	e2 := x1.ShiftAllRight(1).Sub(x3)
	e3 := x1.Add(x3.ShiftAllRight(1))
	return e0.Add(e3), e1.Add(e2), e1.Sub(e2), e0.Sub(e3)
}

// addRow8 adds eight residuals to plane[off:off+8] with unsigned 8-bit saturation.
func addRow8(plane []byte, off int, res archsimd.Int32x8) {
	p := plane[off : off+8 : off+8]
	px := archsimd.Uint64x2{}.SetElem(0, binary.LittleEndian.Uint64(p)).AsUint8x16().ExtendLo8ToUint32().AsInt32x8()
	s := px.Add(res)
	// VPACKUSDW clamps <0 to 0; VPMINUW caps at 255; VPSHUFB gathers the low bytes.
	u := s.GetLo().SaturateToUint16Concat(s.GetHi()).Min(archsimd.BroadcastUint16x8(255))
	out := u.AsUint8x16().PermuteOrZero(packLowBytes).AsUint64x2().GetElem(0)
	binary.LittleEndian.PutUint64(p, out)
}

// join2 loads row i/4 of a into the low lane and of b into the high lane.
func join2(a, b *[16]int32, i int) archsimd.Int32x8 {
	return archsimd.Int32x8{}.SetLo(archsimd.LoadInt32x4Array((*[4]int32)(a[i : i+4]))).
		SetHi(archsimd.LoadInt32x4Array((*[4]int32)(b[i : i+4])))
}

func archHasAVX512() bool { return archsimd.X86.AVX512() }

// idctAdd4x4QuadAVX512 is the AVX-512 tier of idctAdd4x4PairSIMD: four adjacent
// 4×4 blocks (one macroblock row of luma blocks) per Int32x16, one 128-bit lane
// per block, so each transformed row is 16 contiguous output pixels written with
// one load, one VPMOVUSDB narrow and one store.
func idctAdd4x4QuadAVX512(plane []byte, off, stride int, q *[4][16]int32) {
	r0 := join4(q, 0)
	r1 := join4(q, 4)
	r2 := join4(q, 8)
	r3 := join4(q, 12)
	c0, c1, c2, c3 := transpose4x4x4(r0, r1, r2, r3)
	f0, f1, f2, f3 := butterfly4x16(c0, c1, c2, c3)
	g0, g1, g2, g3 := transpose4x4x4(f0, f1, f2, f3)
	h0, h1, h2, h3 := butterfly4x16(g0, g1, g2, g3)
	round := archsimd.BroadcastInt32x16(32)
	addRow16(plane, off, h0.Add(round).ShiftAllRight(6))
	addRow16(plane, off+stride, h1.Add(round).ShiftAllRight(6))
	addRow16(plane, off+2*stride, h2.Add(round).ShiftAllRight(6))
	addRow16(plane, off+3*stride, h3.Add(round).ShiftAllRight(6))
	archsimd.ClearAVXUpperBits()
}

func join4(q *[4][16]int32, i int) archsimd.Int32x16 {
	return archsimd.Int32x16{}.SetLo(join2(&q[0], &q[1], i)).SetHi(join2(&q[2], &q[3], i))
}

func transpose4x4x4(r0, r1, r2, r3 archsimd.Int32x16) (archsimd.Int32x16, archsimd.Int32x16, archsimd.Int32x16, archsimd.Int32x16) {
	t0 := r0.InterleaveLoGrouped(r1).AsInt64x8()
	t1 := r0.InterleaveHiGrouped(r1).AsInt64x8()
	t2 := r2.InterleaveLoGrouped(r3).AsInt64x8()
	t3 := r2.InterleaveHiGrouped(r3).AsInt64x8()
	return t0.InterleaveLoGrouped(t2).AsInt32x16(),
		t0.InterleaveHiGrouped(t2).AsInt32x16(),
		t1.InterleaveLoGrouped(t3).AsInt32x16(),
		t1.InterleaveHiGrouped(t3).AsInt32x16()
}

func butterfly4x16(x0, x1, x2, x3 archsimd.Int32x16) (archsimd.Int32x16, archsimd.Int32x16, archsimd.Int32x16, archsimd.Int32x16) {
	e0 := x0.Add(x2)
	e1 := x0.Sub(x2)
	e2 := x1.ShiftAllRight(1).Sub(x3)
	e3 := x1.Add(x3.ShiftAllRight(1))
	return e0.Add(e3), e1.Add(e2), e1.Sub(e2), e0.Sub(e3)
}

func addRow16(plane []byte, off int, res archsimd.Int32x16) {
	p := plane[off : off+16 : off+16]
	s := archsimd.LoadUint8x16(p).ExtendToUint32().AsInt32x16().Add(res)
	s.Max(archsimd.Int32x16{}).AsUint32x16().SaturateToUint8().Store(p)
}

// idctAdd4x4BlockSIMD is the single-block AVX2 kernel for the sequential I4x4
// path: transform d, add pred (4 bytes per row at predOff, predStride apart)
// and store the clipped result at plane[off:], stride apart. The transform runs
// on four Int32x4 rows (128-bit); rows are paired into Int32x8 for the
// add/clip/pack so the store path matches idctAdd4x4PairSIMD.
func idctAdd4x4BlockSIMD(plane []byte, off, stride int, pred []byte, predOff, predStride int, d *[16]int32) {
	r0 := archsimd.LoadInt32x4Array((*[4]int32)(d[0:4]))
	r1 := archsimd.LoadInt32x4Array((*[4]int32)(d[4:8]))
	r2 := archsimd.LoadInt32x4Array((*[4]int32)(d[8:12]))
	r3 := archsimd.LoadInt32x4Array((*[4]int32)(d[12:16]))
	c0, c1, c2, c3 := transpose4x4(r0, r1, r2, r3)
	f0, f1, f2, f3 := butterfly4x4(c0, c1, c2, c3)
	g0, g1, g2, g3 := transpose4x4(f0, f1, f2, f3)
	h0, h1, h2, h3 := butterfly4x4(g0, g1, g2, g3)
	round := archsimd.BroadcastInt32x8(32)
	lo := archsimd.Int32x8{}.SetLo(h0).SetHi(h1).Add(round).ShiftAllRight(6)
	hi := archsimd.Int32x8{}.SetLo(h2).SetHi(h3).Add(round).ShiftAllRight(6)
	addTwoRows4(plane, off, stride, pred, predOff, predStride, lo)
	addTwoRows4(plane, off+2*stride, stride, pred, predOff+2*predStride, predStride, hi)
	archsimd.ClearAVXUpperBits()
}

func transpose4x4(r0, r1, r2, r3 archsimd.Int32x4) (archsimd.Int32x4, archsimd.Int32x4, archsimd.Int32x4, archsimd.Int32x4) {
	t0 := r0.InterleaveLo(r1).AsInt64x2()
	t1 := r0.InterleaveHi(r1).AsInt64x2()
	t2 := r2.InterleaveLo(r3).AsInt64x2()
	t3 := r2.InterleaveHi(r3).AsInt64x2()
	return t0.InterleaveLo(t2).AsInt32x4(),
		t0.InterleaveHi(t2).AsInt32x4(),
		t1.InterleaveLo(t3).AsInt32x4(),
		t1.InterleaveHi(t3).AsInt32x4()
}

func butterfly4x4(x0, x1, x2, x3 archsimd.Int32x4) (archsimd.Int32x4, archsimd.Int32x4, archsimd.Int32x4, archsimd.Int32x4) {
	e0 := x0.Add(x2)
	e1 := x0.Sub(x2)
	e2 := x1.ShiftAllRight(1).Sub(x3)
	e3 := x1.Add(x3.ShiftAllRight(1))
	return e0.Add(e3), e1.Add(e2), e1.Sub(e2), e0.Sub(e3)
}

// addTwoRows4 adds res (row y in the low lane, row y+1 in the high lane) to two
// 4-byte prediction rows and writes two 4-byte output rows.
func addTwoRows4(plane []byte, off, stride int, pred []byte, poff, pstride int, res archsimd.Int32x8) {
	p0 := pred[poff : poff+4 : poff+4]
	p1 := pred[poff+pstride : poff+pstride+4 : poff+pstride+4]
	pv := uint64(binary.LittleEndian.Uint32(p0)) | uint64(binary.LittleEndian.Uint32(p1))<<32
	px := archsimd.Uint64x2{}.SetElem(0, pv).AsUint8x16().ExtendLo8ToUint32().AsInt32x8()
	s := px.Add(res)
	u := s.GetLo().SaturateToUint16Concat(s.GetHi()).Min(archsimd.BroadcastUint16x8(255))
	out := u.AsUint8x16().PermuteOrZero(packLowBytes).AsUint64x2().GetElem(0)
	o0 := plane[off : off+4 : off+4]
	o1 := plane[off+stride : off+stride+4 : off+stride+4]
	binary.LittleEndian.PutUint32(o0, uint32(out))
	binary.LittleEndian.PutUint32(o1, uint32(out>>32))
}
