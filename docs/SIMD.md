# SIMD in go-media

## Current state

- Production ships **`-tags purego` + `default.pgo`**. Every kernel has a portable Go implementation that is the bit-exact reference.
- **No SIMD kernels are implemented right now.** `KernelsUseSIMD` (`internal/kernels/h264/simd_cpu.go`) is the one-time gate for future kernels: `GOEXPERIMENT=simd` linked, AVX2 or ASIMD present, and `GOMEDIA_ENABLE_SIMD=1` (or `-tags gomedia_simd`). It is resolved once at init; kernels must read it as a plain bool, never re-read the environment or CPU flags per call.
- Report tier 3 (`purego_pgo_simd`) therefore currently measures a `GOEXPERIMENT=simd` build with no kernels enabled.

## Why the first SIMD kernels were removed

The initial kernels (4×4/8×8 IDCT, dequant, recon add, intra V/H, YCbCr row) were slower than scalar. Measured on amd64 with Go 1.27 `GOEXPERIMENT=simd`: 4×4 IDCT about 240 ns SIMD against about 17 ns portable. `go tool objdump` showed:

1. The size-agnostic `simd` package dispatched on vector width per call and copied the 16×int32 array in and out of an `@simd128` clone.
2. The butterfly ran in scalar registers, then round-tripped through `LoadInt32sPart`/`StorePart`, so vectors were used only for the final `(x+32)>>6`.
3. The column pass gathered and scattered with indexed scalar moves instead of shuffles.
4. Every call did a locked atomic increment (a usage probe) and, before dispatch, a map lookup and `os.Getenv`.

The scalar amd64 assembly (`ycbcr_amd64.s`, the 4×4 reconstruct) was also removed: it had no vector instructions and ran one call per pixel or 4×4 block.

## Rules for new kernels

- **Batch.** One call per row, plane or macroblock, never per pixel or per 4×4 block. Take `*[N]T` array pointers rather than by-value arrays, and reslice once before a loop so bounds checks drop out.
- **Use `simd/archsimd` fixed-width types** (`Uint8x16`, `Int16x8`, `Int32x4`; AVX2 256-bit only where a benchmark proves it), not the portable `simd` package. Keep vectors as local values.
- **Tails** shorter than a vector go to the portable code.
- **Bit-exact.** Parity tests and a fuzz test against the portable version, covering odd widths and strides and extreme values. Rounding must match exactly (`(a+b+c+d+2)>>2` is widen, add, shift, narrow; chained `Average` is not the same).
- **Enable only on evidence.** A kernel is switched on only when its micro-benchmark is clearly faster than the portable version (at least about 1.5×) and tier 3 beats tier 2 on the relevant thumbnail assets (`bash scripts/print-three-tier-table.sh`).

## Where the time is

See the stage breakdown in `docs/PROFILING.md`. Entropy decoding (CABAC residual parse, about 39% of a Main/High 1080 thumbnail before the engine rewrite) is serial and cannot be vectorized; dequant, IDCT, prediction and add/clip together are about 15–25%. The bigger remaining levers are overlapping parse with reconstruction across cores and the `image/jpeg` encoder (about 12% of an H.264 thumbnail), with SIMD kernels (halve/area scale, color rows, batched per-macroblock IDCT and prediction, MJPEG IDCT) after those.

## Reporting

- `make compare-gate` writes `test/compare/report_site/data/report.json` (`buildModeTimings`).
- `bash scripts/print-three-tier-table.sh` prints raw ms and Δ% vs ffmpeg per tier.
