# H.264 Baseline (CAVLC) stage profiling

**Decoder base:** mgvs/go-openh264 BSD-2 port under `internal/h264/cleanroom` (see `THIRD_PARTY_NOTICES.md`). Production thumbnails use **purego + PGO**; SIMD is experimental (`GOMEDIA_ENABLE_SIMD=1` + `GOEXPERIMENT=simd`).

> **Correction (Oct 2026):** the timings and the "roughly 2× ffmpeg" conclusion below were measured while the CAVLC decoder produced wrong output: `residualBlockCAVLC` reused a scratch `runs` buffer without clearing it, so run_before values leaked between blocks (decode PSNR about 13 dB against ffmpeg, wrong from macroblock 0). That is fixed, and Baseline is now bit-exact with ffmpeg (`TestBaselineCAVLCMatchesFFmpeg`). The harness now decodes Baseline with the profile restriction lifted on every run and classifies it from the measurement instead of a hardcoded expectation (see `checkThumbnailPreset` in `test/compare/harness/compare.go`). Re-measure on the gate machine before relying on the tables below.

## How to reproduce

```bash
CGO_ENABLED=0 go test -run=^$ -bench=BaselineThumbnail -benchmem ./internal/h264/
bash scripts/baseline-tier-report.sh   # amd64 tier timings (requires compare assets)
make compare-gate   # purego+PGO harness; three-tier table in report + scripts/print-three-tier-table.sh
```

Compare against ffmpeg thumbnail timing from `make compare-gate` (`mp4_h264_baseline_1080` remains **typed_unsupported** until gate passes).

## Gate policy

- **Baseline is enabled** (Oct 2026): bit-exact with ffmpeg and within the ≤20% timing gate on the measured machine (arm64, Homebrew ffmpeg 7.1.5: about 33% and 49% faster for large and small). Its `mp4_h264_baseline_1080` thumbnail rows are `supported` in `expectations.json`, so `make compare-gate` enforces the gate on them. Confirm on the amd64 gate machine with ffmpeg 9.0.2.
- If it ever misses the gate, add `66: "<reason>"` to `gatedProfiles` in `internal/h264/sps.go` and set the rows back to `typed_unsupported`; the harness then reports the measured gap (`performance:`) and fails with "enable it" when it passes again.

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
| Shipped (in-process PGO), historical | asm + PGO (the scalar asm has since been removed) | **~100** | **~92%** |
| 1 · purego | `-tags purego` | **~104** | **~100%** |
| 2 · purego+PGO | tier 1 + `default.pgo` | **~103** | **~98%** |
| 3 · purego+PGO+simd | tier 2 + `GOEXPERIMENT=simd` | **~102** | **~96%** |

ffmpeg median for the same asset/preset (harness seek, large preset): **~52 ms** (15-run median on static ffmpeg 9.0.2).

**Gate outcome:** Baseline remains **unsupported** — decode+scale+JPEG is still roughly **2×** ffmpeg after allocation fixes; entropy + full-frame decode dominate, not 4×4 IDCT.

### SIMD

The first SIMD kernels (4×4/8×8 IDCT, dequant, recon, intra V/H, YCbCr row) were slower than scalar (4×4 IDCT about 248 ns against about 26 ns portable) and have been removed. See `docs/SIMD.md` for why and for the rules new kernels follow. Tier-3 timings above were measured with those kernels and are historical.

### Next levers

| Stage | Status |
|-------|--------|
| CAVLC entropy | Allocations largely fixed; further wins need coeff batching / zero-block fast path |
| Intra plane / 8×8 pred SIMD | Portable mgvs |
| CABAC vector helpers | TBD |
