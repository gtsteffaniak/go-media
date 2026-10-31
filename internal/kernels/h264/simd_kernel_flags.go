package h264

// Per-kernel SIMD enables, chosen by measurement (govpx-style static defaults).
//
// Each flag is a compile-time constant, so a disabled kernel's SIMD branch is
// dead code and the dispatch costs nothing: no map lookup, no atomic, and the
// portable body stays inlinable. A kernel is switched on only when its
// BenchmarkKernel* pair shows the SIMD body beating portable on the box CPU
// (see docs/SIMD.md for the numbers). There is no startup self-benchmark.
//
// The runtime guard KernelsUseSIMD (experiment linked + AVX2/ASIMD present +
// not disabled via GOMEDIA_DISABLE_SIMD) is still checked at every call site.
const (
	simdIDCT4x4    = false // scalar butterflies wrapped in vector load/store; loses to portable
	simdIDCT8x8    = false
	simdDequant4x4 = false
	simdRecon4x4   = false
	simdIntra4x4   = false // 4-byte copies; portable already compiles to single moves
	simdIntra16x16 = false
	simdIDCTAdd16  = false
	// simdIDCTAddPair: two adjacent 4×4 blocks per AVX2 register, transform+add+clip
	// fused; 6.8× the portable pair on Xeon AVX2 (11.7 vs 80 ns). amd64 archsimd only.
	simdIDCTAddPair = simdArchKernels
	// simdIDCTAddQuad: AVX-512 tier, four blocks (one MB row) per Int32x16.
	simdIDCTAddQuad = simdArchKernels
	// arm64: no kernel yet. Go 1.27 archsimd on arm64 has no vector reinterpret
	// (AsInt64x2) so the 4×4 transpose cannot be expressed; arm64 runs purego+PGO.
)

// simdKernels lists every kernel with its static enable, for reporting.
var simdKernels = []struct {
	name string
	on   bool
}{
	{"idct4x4", simdIDCT4x4},
	{"idct8x8", simdIDCT8x8},
	{"dequant4x4", simdDequant4x4},
	{"recon4x4", simdRecon4x4},
	{"intra4x4", simdIntra4x4},
	{"intra16x16", simdIntra16x16},
	{"idct_add16", simdIDCTAdd16},
	{"idct_add_pair", simdIDCTAddPair},
	{"idct_add_quad_avx512", simdIDCTAddQuad},
}

// KernelSIMD reports whether a named kernel dispatches to its SIMD body.
// Reporting only; hot paths test the constants directly.
func KernelSIMD(name string) bool {
	if !KernelsUseSIMD {
		return false
	}
	for _, k := range simdKernels {
		if k.name == name {
			return k.on
		}
	}
	return false
}

func anyKernelSIMDEnabled() bool {
	for _, k := range simdKernels {
		if k.on {
			return true
		}
	}
	return false
}

// SIMDYCbCrRow enables the YCbCr area-downscale row kernel in internal/kernels.
const SIMDYCbCrRow = false
