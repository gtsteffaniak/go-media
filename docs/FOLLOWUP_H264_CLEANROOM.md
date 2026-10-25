# Follow-up: clean-room H.264 intra decoder

**Tracked for a stacked PR after thumbnail/PGO/SIMD reporting (#8).**

## Goal

Remove **`third_party/hi264`** and **`replace github.com/Eyevinn/hi264`**. Implement intra-only **Main/High** decode entirely in **`internal/h264`**, written from **ITU-T H.264** with **ffmpeg** and **openh264** as behavioral references only (no GPL/LGPL source copies; MIT-compatible).

## Must keep

- Support matrix: Main/High 8-bit 4:2:0 CABAC, 8×8 transform, scaling lists; **Baseline unsupported** unless it passes the ≤20% ffmpeg gate.
- Bit-exact **YUV vs ffmpeg** on harness golden fixtures.
- **`mp4ff/avc`** (or inlined equivalent) only if still the smallest correct SPS/PPS/slice parser — otherwise local parsers in `internal/h264`.

## SIMD axis

- IDCT/dequant, intra prediction, residual batches, optional deblock, YCbCr downscale, JPEG path — implement **simd / archsimd** variants with asm + purego parity.

## Acceptance

- `go mod tidy` with no `Eyevinn/hi264`.
- `make compare-gate` green; report build modes unchanged in meaning.
