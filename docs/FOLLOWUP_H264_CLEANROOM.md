# Follow-up: clean-room H.264 intra decoder

## Status (PR #9)

Relocated **Eyevinn/hi264** under `internal/h264/{intradec,cabac,…}` is **unacceptable** and must be deleted once `internal/h264/cleanroom` passes parity and timing gates. See commit `7995631` lineage in PR #9 description.

## Rewrite goals (Graham)

- **Full rewrite** from **ITU-T H.264** (ffmpeg/openh264 behavioral references only; **no hi264** in any form).
- **SIMD-first:** `GOEXPERIMENT=simd` / archsimd kernels with a portable Go fallback (`-tags purego` forces it); bit-identical on shared kernel tests.
- **Baseline (CAVLC):** design to pass **≤20%** ffmpeg thumbnail gate; **profile per stage** (demux/NAL, slice syntax, CAVLC entropy, coeff batching, dequant/IDCT, intra pred, plane writes). Gate passed (Oct 2026) → **supported**..
- **Main/High (CABAC):** decode-only intra; bit-exact YUV vs ffmpeg on harness fixtures.
- **Thumbnails:** **skip in-loop deblock** on fast path; full deblock optional for parity tests.
- **Hot path:** **zero allocations** per frame (pooled bitstream state, coeff batches, frame buffers); **PGO** via checked-in `default.pgo`.

## Architecture (`internal/h264/cleanroom`)

| Package | Role |
|---------|------|
| `bitstream` | **64-bit bit cache** RBSP reader (UE/SE/fixed, `more_rbsp_data`) |
| `cavlc` | **Table-driven CAVLC**; multi-symbol coeff_token / total_zeros / run_before LUTs |
| `cabac` | CABAC decode-only (no encoder) |
| `coeff` | Batched residual levels (luma/chroma, 4×4 / 8×8) |
| `intrapred` | 16×16 / 4×4 / 8×8 / chroma modes |
| `pix` | Pooled Y/Cb/Cr planes + crop metadata |
| `decode` | NAL AU orchestration; `SkipDeblock` for thumbs |

SIMD lives in **`internal/kernels/h264`**: dequant+IDCT+reconstruct, intra pred helpers, 4×4/8×8 plane writes.

## Gates (unchanged)

- `make compare-gate` green; report **buildModeTimings** tiers purego → purego+PGO → purego+PGO+simd (gate uses **in-process asm+PGO** only). Regenerate **`default.pgo`** after rewrite (`make pgo`).
- Baseline is enabled. Profiles that miss the gate are listed in `gatedProfiles` in `sps.go`.
