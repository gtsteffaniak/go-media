# SIMD (`GOEXPERIMENT=simd`)

## Production vs experiment

| Build | Kernels |
|-------|---------|
| **Production** (`CGO_ENABLED=0` + `-pgo=default.pgo`) | Portable H.264 DSP; YCbCr asm on amd64/arm64. |
| **Compare tier 3** (not production) | Production build + `GOEXPERIMENT=simd` at link time. |
| **Tests** | `CGO_ENABLED=0 GOEXPERIMENT=simd go test ./internal/kernels/h264/ -run TestSIMD` |

Dispatch (`internal/kernels/h264/simd_cpu.go`):

- **`KernelsUseSIMD`:** experiment + AVX2 (amd64) or ASIMD (arm64), and not `GOMEDIA_DISABLE_SIMD=1`.
- **`KernelSIMD(name)`:** per-kernel map in `simd_kernel_flags.go` (H.264 kernels default **on**; `ycbcr_row` off until rounding matches portable).
- **`media.SIMDActive()`:** `KernelsUseSIMD` and at least one kernel flag enabled.

Bodies live in `*_simd.go` (`goexperiment.simd`). They must match portable on shared tests.

## Expectations

On Baseline 1080 thumbs, transform is ~10% of decode — SIMD will not close the amd64 Baseline gate by itself ([H264.md](H264.md)). Tier 3 should be tracked in `buildModeTimings`; enable or replace kernels when micro-bench and tier 3 beat tier 2 on relevant assets.

**Rules for new work:**

- Batch per row, plane, or macroblock — not per pixel or per 4×4.
- Prefer `simd/archsimd` for shuffle-heavy IDCT; avoid scalar butterflies + one `simd.Int32s` round-trip per block.
- Bit-exact vs portable; fuzz odd strides.
- SoA / batched `idct_add16` not implemented; early experiment kernels were slower than portable on 4×4 micro-benches until redesigned.

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
