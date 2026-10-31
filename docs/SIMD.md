# SIMD (`GOEXPERIMENT=simd`)

[← Back to README](../README.md) · [Examples](../examples/README.md) · [Live report](https://gtsteffaniak.github.io/go-media/)

> [!WARNING]
> SIMD kernels are experimental and off in production builds. Ship `CGO_ENABLED=0 go build -pgo=default.pgo`.

## Production vs experiment

| Build | Kernels |
|-------|---------|
| **Production** (`CGO_ENABLED=0` + `-pgo=default.pgo`) | Portable H.264 DSP; YCbCr asm on amd64/arm64. |
| **Compare tier 3** (not production) | Production build + `GOEXPERIMENT=simd` at link time. |
| **Tests** | `CGO_ENABLED=0 GOEXPERIMENT=simd go test ./internal/kernels/h264/ -run TestSIMD` |

Dispatch (`internal/kernels/h264/simd_cpu.go`, `simd_kernel_flags.go`), modelled on govpx's `internal/pixel`:

- **Static per-kernel enables.** Each kernel has a compile-time constant (`simdIDCTAddPair`, `simdIDCTAddQuad`, …) set from measurements. A disabled kernel is dead code: no map lookup, no atomic, and the portable body stays inlinable. There is no startup self-benchmark.
- **`KernelsUseSIMD`:** experiment linked, AVX2 present (amd64) or ASIMD (arm64), and not `GOMEDIA_DISABLE_SIMD=1`.
- **`KernelsUseAVX512`:** `KernelsUseSIMD` plus `archsimd.X86.AVX512()`; `GOMEDIA_DISABLE_AVX512=1` falls back to the AVX2 tier. Selects the tiers below at runtime: AVX-512, then AVX2, then portable.
- **`media.SIMDActive()`:** `KernelsUseSIMD` and at least one kernel enabled.

| Kernel | Tier | Layout | Result (Xeon, ns/op) |
|--------|------|--------|----------------------|
| `idct_add_pair` | AVX2 | two adjacent 4×4 blocks per `Int32x8`, one block per 128-bit lane; transpose, butterfly, round, add and clip fused | 11.5 vs 85 portable (7×) |
| `idct_add_quad_avx512` | AVX-512 | four blocks (one macroblock row) per `Int32x16`; one 16-byte store per row | 18 vs 23 for two AVX2 pairs, 175 portable |
| everything else (`idct4x4`, `idct8x8`, `dequant4x4`, `recon4x4`, `intra*`, `idct_add16`, `ycbcr_row`) | off | scalar butterflies around one vector load/store per block; lost to portable | n/a |

arm64 has no kernel: Go 1.27's `archsimd` on arm64 cannot reinterpret `Int32x4` as 64-bit lanes (no `AsInt64x2`), which the 4×4 transpose needs, so M-series chips run the purego+PGO path.

`TestSIMDKernelsAVX2Only` disassembles the AVX2-tier kernels and fails on any AVX-512 instruction, because `KernelsUseSIMD` only checks AVX2.

Bodies live in `*_amd64.go` (`goexperiment.simd && amd64 && !purego`) with `*_other.go` fallbacks, and must match portable on shared tests.

## Expectations

On Baseline 1080 thumbs, transform is ~6% of decode — SIMD will not close the amd64 Baseline gate by itself ([H264.md](H264.md)). Tier 3 should be tracked in `buildModeTimings`; enable or replace kernels when micro-bench and tier 3 beat tier 2 on relevant assets.

**Rules for new work:**

- Batch per row, plane, or macroblock — not per pixel or per 4×4.
- Prefer `simd/archsimd` for shuffle-heavy IDCT; avoid scalar butterflies + one `simd.Int32s` round-trip per block.
- Bit-exact vs portable; fuzz odd strides.
- Only AVX2 instructions in the AVX2 tier; AVX-512-only methods (`TruncTo*`, `Uint8x16.Permute`, `SaturateTo*` on 512-bit) belong in the explicit AVX-512 kernels.
- A kernel is enabled only when its micro-benchmark and the tier 2 vs tier 3 thumbnail benchmark both win.

## CI / debugging

| Case | Command |
|------|---------|
| Kernel tests | `GOEXPERIMENT=simd go test ./internal/kernels/h264/ -run TestSIMD` |
| Opt-out | `GOMEDIA_DISABLE_SIMD=1` |
| Emulation off | `GODEBUG=simd=0` (parity when kernels on) |

Report tiers: `make compare-gate` → `report.json` `buildModeTimings`; `bash scripts/print-three-tier-table.sh`.

## References

- [Go simd experiment](https://go.dev/blog/simd) — `GOEXPERIMENT=simd`, `GODEBUG=simd=0`
- [PROFILING.md](PROFILING.md) — gate vs tiers, pprof commands
