# SIMD / CAVLC follow-up plan (post-merge)

Production ships **`-tags purego` + `default.pgo`**. Runtime SIMD is **off** unless `GOMEDIA_ENABLE_SIMD=1` (or `-tags gomedia_simd` for experiment binaries). Each kernel has a **static false** default in `internal/kernels/h264/simd_kernel_flags.go`; flip only after benchmarks beat purego+PGO end-to-end.

## 1. Diagnose 4×4 SIMD slowness (before rewriting)

See `docs/SIMD_IDCT_DIAGNOSIS.md`. Summary:

- `go tool objdump` on `inverseTransform4x4ResidualSIMD`: archsimd width dispatch, 64-byte stack copies, scalar butterfly + vector load/store round-trip, scalar column transpose, probe atomic.
- pprof micro-benches; do not trust “vmov exists” tests alone.

## 2. Batched SoA 16×4×4 IDCT (future)

- Structure-of-arrays: one vector lane per block, one vector per coefficient index; no transpose.
- Wire after MB coeff parse; serial predict+add for intra neighbors.
- Benchmark vs portable per 16 blocks; if it cannot beat portable SoA, **drop transform SIMD**.

## 3. Amdahl — Baseline / CAVLC first

1080p Baseline IDR profile (~65 ms/op, ~13 allocs after RBSP/CAVLC pooling):

| Area | ~cum% |
|------|-------|
| `decodeMBNxN` + `residualBlockCAVLC` | dominant |
| `readBit` / `refillReg` | ~15% |
| `readLevelsInto`, coeff_token, run_before | ~15% |
| dequant + IDCT | smaller |

Prioritize: table-driven coeff_token/level/run_before, bit-cache refill, branch reduction — not transform vectorization until entropy is addressed.

## 4. Per-kernel gating

Enable in `simd_kernel_enabled` only when tier-3 thumb subprocess beats tier-2 for relevant assets. **YCbCr row downscale** is the first candidate to re-test. No runtime self-benchmark at startup.

## 5. Reporting

- `make compare-gate` → `test/compare/report_site/data/report.json` (`buildModeTimings`).
- `bash scripts/print-three-tier-table.sh` for raw ms + Δ% vs ffmpeg.
