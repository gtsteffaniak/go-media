# Follow-up: clean-room H.264 intra decoder

## Status (PR #9, branch `cursor/h264-cleanroom-decode-5c2e`)

**PR #9 is not acceptable as shipped.** Commit `7995631` relocated the former `third_party/hi264` tree into `internal/h264` with the same package layout (`intradec`, `cabac` including `encoder.go`, `cavlc`, `frame`, `h264context`, `pred`, `reconstruct`, `slice`, `transform`, `yuv`). That is **Eyevinn/hi264 in another directory**, not a clean-room decoder.

### Honest lineage (current tree before replacement)

| Area | Approx. lines | Origin |
|------|---------------|--------|
| `intradec`, `cabac`, `cavlc`, `slice`, `transform`, `pred`, `reconstruct`, `h264context`, `yuv` | ~8,262 | **100%** relocated from vendored **Eyevinn/hi264** (`7995631`) |
| `frame` | ~125 | Same relocation (hi264 `pkg/frame`) |
| `testdata/golden/*.264` | 40 bitstreams | hi264 encoder regression corpus |
| `reconstruct/reconstruct_amd64.s` | asm | hi264 relocation |
| Façade (`h264.go`, `decoder_pool.go`, `sps.go`, `avcc.go`, `rgb.go`, `yuv420.go`, `thumb_ycc.go`) | ~450 | Written for go-media; **calls** the hi264-derived `intradec` stack |

**Bottom line:** essentially **all entropy, syntax, prediction, transform, and reconstruct code** in PR #9 is hi264-derived. Only SPS policy, MP4 wiring, RGB/YUV helpers, and tests are go-media-specific.

### Goal (unchanged)

Remove hi264 in **any** form (module, vendor, or relocated copy). Implement **decode-only** intra **Main/High** 8-bit 4:2:0 CABAC in **`internal/h264/cleanroom`** (or `decode/`) from **ITU-T H.264**, with **ffmpeg** / **openh264** as behavioral references only (no GPL/LGPL copies; no hi264 source).

### Must keep

- Baseline **unsupported** at SPS (`ErrUnsupportedProfile`) unless a future CAVLC path passes ≤20% vs ffmpeg.
- Bit-exact **YUV vs ffmpeg** on harness golden fixtures; **≤20%** thumbnail gate on **default** in-process timings.
- Comparison report **buildModeTimings**: default / `purego` / `GOEXPERIMENT=simd`.
- **`mp4ff/avc`** for SPS/PPS/slice header parsing where it remains the smallest correct choice.

### SIMD

- IDCT / residual add, intra pred hot paths, optional deblock — **amd64 asm** + **`-tags purego`** + optional **simd** experiment; bit-identical on shared tests.

### Acceptance

- No `Eyevinn/hi264` in `go.mod`, `go.sum`, or tree.
- No `cabac/encoder.go` or other encoder paths.
- `make compare-gate` green; CI green; CodeRabbit threads resolved.
