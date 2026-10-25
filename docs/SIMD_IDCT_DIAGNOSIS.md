# SIMD 4×4 IDCT diagnosis (amd64, Go 1.27 `GOEXPERIMENT=simd`)

Recorded for the follow-up refactor; **not** enabled in production (see `docs/SIMD_FOLLOWUP_PLAN.md`).

## Symptom

`BenchmarkIDCT4x4Portable` ≈ **17 ns/op** vs `BenchmarkIDCT4x4SIMD` ≈ **240 ns/op** (~14× slower).

## `go tool objdump` findings

1. **Archsimd wrapper** — `inverseTransform4x4ResidualSIMD` dispatches on `simd.maxVectorSize`, copies 16×int32 with `MOVUPS` before/after `CALL …@simd128`.
2. **Scalar butterfly** — `butterfly1DInt32s@simd128` computes in GPRs, then `LoadInt32sPart` / `StorePart` (no vector math on the butterfly).
3. **Scalar transpose** — column pass gathers/scatters with indexed `MOVL` (no shuffles).
4. **Probe** — `LOCK XADD` on `SIMDProbeCounts` at entry.
5. **Rounding only** — vector ops used mainly for `(h+32)>>6` on rows.

## Baseline decode profile (context)

`BenchmarkBaselineThumbnailDecode` (~65 ms/op): entropy/CAVLC dominates; IDCT is a small fraction. See follow-up plan for CAVLC priorities.

## Planned next steps

- SoA 16-block batched IDCT experiment; per-kernel static enable only if tier-3 beats tier-2 on real thumbnails.
