# H.264 Baseline (CAVLC) stage profiling

**Decoder base:** mgvs/go-openh264 BSD-2 port under `internal/h264/cleanroom` (see `THIRD_PARTY_NOTICES.md`). Hot IDCT/recon use `internal/kernels/h264`.

## How to reproduce

```bash
CGO_ENABLED=0 go test -run=^$ -bench=BaselineThumbnail -benchmem ./internal/h264/
bash scripts/baseline-tier-report.sh   # amd64 tier timings (requires compare assets)
```

Compare against ffmpeg thumbnail timing from `make compare-gate` (`mp4_h264_baseline_1080` remains **typed_unsupported** until gate passes).

## Gate policy

- If Baseline **large/small** presets are **≤20%** vs ffmpeg 9.0.2 on the **shipped in-process build** (amd64 kernels + `-pgo=default.pgo`) → enable Baseline in `validateSPSNAL` and add `mp4_h264_baseline_1080` to `timingGate.enforcedAssetIds`.
- Otherwise keep `ErrUnsupportedProfile` (current policy).

## Measured results (amd64 cloud VM, cleanroom + kernels, Oct 2026)

### Micro-bench (decode only, `fast=true`, skip deblock)

| Benchmark | ns/op (≈) | B/op | allocs/op |
|-----------|-----------|------|-----------|
| `BenchmarkBaselineThumbnailDecode` | **~65 ms** | **~15.7 MB** | **~187k** |
| `BenchmarkBaselineThumbnailDecodeFullDeblock` | **~80 ms** | **~12.9 MB** | **~187k** |

### Full thumbnail `thumbnail_large` (go+write median ms, temporary Baseline enable for measurement)

| Tier | Build | go+write ms | Δ% vs ffmpeg (~52 ms) |
|------|-------|-------------|-------------------------|
| Shipped (in-process) | asm kernels + PGO | **~104** | **~100%** |
| 1 · purego | `-tags purego` | **~104** | **~100%** |
| 2 · purego+PGO | tier 1 + `default.pgo` | **~103** | **~98%** |
| 3 · purego+PGO+simd | tier 2 + `GOEXPERIMENT=simd` | **~102** | **~96%** |

ffmpeg median for the same asset/preset (harness seek, large preset): **~52 ms** (15-run median on static ffmpeg 9.0.2).

**Gate outcome:** Baseline remains **unsupported** — CAVLC path still allocates ~187k objects per 1080p IDR and decode+scale+JPEG is roughly **2×** ffmpeg, far above the 20% ceiling. SIMD tiers improve only ~2% today because IDCT/recon are not the dominant cost; **entropy + allocations** dominate.

### Profile-backed root cause

1. **Allocations:** ~187k heap allocs per 1080p IDR on the CAVLC path (coeff/residual parsing still allocates on some branches).
2. **Entropy:** CAVLC `residualBlockCAVLC` / `readLevels` volume at 1080p; CABAC-style pooling does not cover all CAVLC paths equally.
3. **Policy:** `validateSPSNAL` rejects Baseline until compare-gate passes on `mp4_h264_baseline_1080` without exemptions.

### Next levers (SIMD-first)

| Stage | Status |
|-------|--------|
| 4×4 IDCT + recon | Wired to `internal/kernels/h264` (portable + simd hook) |
| CAVLC entropy | Bitstream unary/cache; needs batched coeff buffers |
| Intra pred / 8×8 | Portable mgvs; asm/simd TBD |
| Deblock | Skipped on thumbnail fast path |
