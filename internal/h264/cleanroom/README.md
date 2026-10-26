# Clean-room H.264 intra decoder

Full **SIMD-first rewrite** (ITU-T H.264; ffmpeg/openh264 as behavioral references only).

## Design pillars (Graham)

- **64-bit bit cache** — `bitstream/`
- **Table-driven CAVLC** with multi-symbol LUTs — `cavlc/`
- **Batched coefficients** — `coeff/`
- **CABAC decode-only** — `cabac/` (Main/High)
- **Kernels** — `internal/kernels/h264` (dequant/IDCT/reconstruct, intra pred, plane writes; asm + purego + simd)
- **Thumbnails:** skip in-loop deblock on fast path
- **Zero allocations** per frame after pool warm; **PGO** via `default.pgo`
- **Baseline:** profile per stage (`docs/BASELINE_PROFILE.md`); enable in `sps.go` only if ≤20% gate passes

Sibling `intradec`/`cabac`/… packages are **hi264 relocations** — delete when this tree is parity-clean.

See `docs/FOLLOWUP_H264_CLEANROOM.md`.
