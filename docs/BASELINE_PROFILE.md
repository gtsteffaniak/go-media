# H.264 Baseline (CAVLC) stage profiling

**Context:** Until `internal/h264/cleanroom` replaces the relocated hi264 stack, benchmarks use the current façade (`decodeFrame` with `fast=true` = skip deblock for thumbnails).

## How to reproduce

```bash
CGO_ENABLED=0 go test -run=^$ -bench=BaselineThumbnail -benchmem ./internal/h264/
```

Compare against ffmpeg thumbnail timing from `make compare-gate` report (`mp4_h264_baseline_1080`).

## Target design (clean-room)

| Stage | SIMD / zero-alloc notes |
|-------|-------------------------|
| NAL / RBSP | 64-bit bit cache (`cleanroom/bitstream`) |
| CAVLC entropy | Table-driven multi-symbol LUTs (`cleanroom/cavlc`) |
| Coefficients | Batched levels per MB (`cleanroom/coeff`) |
| Dequant + IDCT + reconstruct | `internal/kernels/h264` (asm + purego + simd) |
| Intra pred + plane writes | kernels + pooled `cleanroom/pix` |
| Deblock | **skipped** on thumbnail fast path |

## Gate policy

- If Baseline **large/small** presets are **≤20%** vs ffmpeg 9.0.2 on **default** in-process timings → enable Baseline in `validateSPSNAL`.
- Otherwise keep `ErrUnsupportedProfile` and record **profile-backed** dominant stage here.

## Results (amd64 cloud VM, legacy hi264 relocation stack)

Measured with `go test -run=^$ -bench=BaselineThumbnail -benchmem ./internal/h264/` (3 runs).

| Benchmark | ns/op (≈) | B/op | allocs/op | Notes |
|-----------|-----------|------|-----------|-------|
| `BenchmarkBaselineThumbnailDecode` | **56–58 ms** | **50.5 MB** | **187,336** | `fast=true` (skip deblock) |
| `BenchmarkBaselineThumbnailDecodeFullDeblock` | **70–72 ms** | **50.5 MB** | **187,336** | parity path |

### Profile-backed root cause (pre-cleanroom)

1. **Allocations:** ~187k heap allocs and ~50 MB per 1080p IDR — incompatible with the **zero-allocation** thumbnail goal; dominates vs ffmpeg’s native decoder.
2. **Entropy + syntax:** CAVLC on relocated stack is not table-batched SIMD-first; entropy is a large fraction of frame time (see ~57 ms full-frame bench vs ~15 ms deblock delta).
3. **Policy:** `validateSPSNAL` keeps Baseline **unsupported** until clean-room CAVLC + kernels pass **≤20%** compare-gate on `mp4_h264_baseline_1080` (not merely micro-bench).

**Compare-gate delta % (baseline 1080):** run `make compare-gate` after cleanroom wiring; expect typed unsupported today.

### Clean-room targets

| Metric | Legacy (above) | Target |
|--------|----------------|--------|
| allocs/op | 187k | **0** on hot path (pooled bitstream, coeff batches, planes) |
| B/op | 50 MB | **0** steady-state after pool warm |
| ns/op | ~57 ms | ≤20% over ffmpeg thumbnail go+write on baseline asset |
