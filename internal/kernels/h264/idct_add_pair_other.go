//go:build !(goexperiment.simd && amd64 && !purego)

package h264

// simdArchKernels is false when no archsimd kernel bodies are linked.
const simdArchKernels = false

func idctAdd4x4PairSIMD(plane []byte, off, stride int, a, b *[16]int32) {
	idctAdd4x4PairPortable(plane, off, stride, a, b)
}

func archHasAVX512() bool { return false }

func idctAdd4x4QuadAVX512(plane []byte, off, stride int, q *[4][16]int32) {
	idctAdd4x4QuadPortable(plane, off, stride, q)
}
