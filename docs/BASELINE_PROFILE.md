# H.264 Baseline (CAVLC) stage profiling

**Decoder base:** mgvs/go-openh264 BSD-2 port under `internal/h264/cleanroom` (see `THIRD_PARTY_NOTICES.md`). Hot IDCT/recon/dequant/intra V/H use `internal/kernels/h264` with runtime dispatch (`golang.org/x/sys/cpu` + `GOEXPERIMENT=simd`).

## How to reproduce

```bash
CGO_ENABLED=0 go test -run=^$ -bench=BaselineThumbnail -benchmem ./internal/h264/
CGO_ENABLED=0 GOEXPERIMENT=simd go test ./internal/kernels/h264/ -run TestSIMD
bash scripts/baseline-tier-report.sh   # amd64 tier timings (requires compare assets)
make compare-gate   # PGO-built harness; three-tier rows in report_site/data/report.json
```

Compare against ffmpeg thumbnail timing from `make compare-gate` (`mp4_h264_baseline_1080` remains **typed_unsupported** until gate passes).

## Gate policy

- If Baseline **large/small** presets are **≤20%** vs ffmpeg 9.0.2 on the **shipped in-process build** (amd64 kernels + `-pgo=default.pgo`) → enable Baseline in `validateSPSNAL` and add `mp4_h264_baseline_1080` to `timingGate.enforcedAssetIds`.
- Otherwise keep `ErrUnsupportedProfile` (current policy).

## Measured results (amd64 cloud VM, Oct 2026)

### Micro-bench (decode only, `fast=true`, skip deblock)

| Benchmark | ns/op (≈) | B/op | allocs/op |
|-----------|-----------|------|-----------|
| `BenchmarkBaselineThumbnailDecode` | **~65 ms** | **~3.1 MB** | **~13** |
| `BenchmarkBaselineThumbnailDecodeFullDeblock` | **~79 ms** | **~435 B** | **~7** |

Root cause of the old ~187k allocs/frame was per-NAL `EBSPtoRBSP` append growth; fixed via `EBSPtoRBSPDst` + slice reuse in `au.go` and pooled CAVLC coeff scratch in `cavlc_blocks_sd.go`.

### Full thumbnail `thumbnail_large` (go+write median ms, temporary Baseline enable for measurement)

| Tier | Build | go+write ms | Δ% vs ffmpeg (~52 ms) |
|------|-------|-------------|-------------------------|
| Shipped (in-process PGO) | asm + PGO | **~100** | **~92%** |
| 1 · purego | `-tags purego` | **~104** | **~100%** |
| 2 · purego+PGO | tier 1 + `default.pgo` | **~103** | **~98%** |
| 3 · purego+PGO+simd | tier 2 + `GOEXPERIMENT=simd` | **~102** | **~96%** |

ffmpeg median for the same asset/preset (harness seek, large preset): **~52 ms** (15-run median on static ffmpeg 9.0.2).

**Gate outcome:** Baseline remains **unsupported** — decode+scale+JPEG is still roughly **2×** ffmpeg after allocation fixes; entropy + full-frame decode dominate, not 4×4 IDCT.

### SIMD proof (kernels package)

| Kernel | Probe | Parity tests | Notes |
|--------|-------|--------------|-------|
| 4×4 IDCT + rounding | `ProbeIDCT4x4` | `TestSIMDIDCTParity` | archsimd `Int32s` rounding rows |
| 8×8 IDCT + rounding | `ProbeIDCT8x8` | `TestIDCT8x8Parity` | 8-wide row rounding |
| 4×4 recon add/clamp | `ProbeRecon4x4` | via decode tests | |
| Dequant 4×4 | `ProbeDequant4x4` | `TestDequant4x4Parity` | 4-lane mul/shift when `shift<4` |
| Intra 4×4/16×16 V/H | `ProbeIntra4x4` | predict tests | |
| YCbCr area down row | `ProbeYCbCrRow` | thumb path | |

`TestSIMDKernelsExecuteWhenEnabled` asserts probe hits ≥2; `TestSIMDBinaryContainsVectorOps` greps `objdump` for `vmov` (amd64) / `ldr` (arm64).

Micro-benchmark: 4×4 IDCT SIMD is **slower** than portable on tiny blocks (~248 ns vs ~26 ns) due to vector setup; tier-3 thumbnail timings for **Main/High 1080** remain within gate (decode-bound).

### Next levers

| Stage | Status |
|-------|--------|
| CAVLC entropy | Allocations largely fixed; further wins need coeff batching / zero-block fast path |
| Intra plane / 8×8 pred SIMD | Portable mgvs |
| CABAC vector helpers | TBD |
