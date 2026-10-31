# Decoder integration patterns

Notes for wiring codec libraries into go-media thumbnails (reusable when adding AV1 or extending H.264).

## External codec modules (VP9, HEVC)

- **VP9:** [gtsteffaniak/go-vpx](https://github.com/gtsteffaniak/go-vpx) (`internal/vp9decode`) — libvpx-derived pure Go, bit-exact gate vs libvpx.
- **HEVC:** [gen2brain/h265](https://github.com/gen2brain/h265) (`internal/hevcdecode`) — MIT, pure Go with plan9 `GOARCH` assembly (AVX2/AVX-512 on amd64). Thumbnail path uses `hevc.Decoder` + `heic.DecodeColor` for HEIF stills without embedded JPEG.

Wrap third-party decoders in a thin `internal/*decode` package: map errors to `errs.Unsupported` / `ErrUnsupportedProfile`, convert planes to `h264.ThumbYCbCr`, and keep the public API on `pkg/media` / `internal/thumb`.

## go-vpx SIMD and scalar fast paths

govpx splits DSP into portable reference bodies and arch-specific files (`internal/pixel/*_amd64.go`, `*_arm64.go`, `*_nosimd.go`). **Static per-kernel enables** (see go-media `internal/kernels/h264/simd_kernel_flags.go`, modelled on govpx): each hot kernel is a compile-time constant; disabled kernels stay inlinable portable Go with no runtime map lookup.

**Scalar fast paths** (e.g. `internal/vp9/dsp/convolve_unscaled.go`): when SIMD is off, unscaled 8-tap filters pick taps once per block, pre-slice rows to drop bounds checks, unroll taps, and walk the vertical pass row-major instead of column-major — bit-identical to the generic convolve.

**Testing:** `*_simd_reference_test.go` and `*_disabled_scalar_*_test.go` compare SIMD vs portable on shared inputs; VP9 gate fixtures hash every frame against libvpx/ffmpeg.

## go-media H.264 cleanroom + `GOEXPERIMENT=simd`

Production: portable DSP + `-pgo=default.pgo`. Tier 3 (diagnostic): `GOEXPERIMENT=simd` with AVX2 then AVX-512 tiers where micro-benchmarks win (`docs/SIMD.md`). HEVC assembly lives in the **h265 module**, not go-media’s simd experiment — compare tier 2 still benefits from PGO on the wrapper and YCbCr downscale.

## Container wiring

| Container | Seek | Extradata | Sample |
|-----------|------|-----------|--------|
| MP4/MOV | `stss` sync sample at seek% | `hvcC` / `avcC` in `stsd` | Length-prefixed NALs |
| MKV | Cluster walk to keyframe ≤ target time | `CodecPrivate` (hvcC) | Length-prefixed or lace demux |
| HEIC | Primary item (no seek) | `hvcC` on item | `iloc` extent; JPEG `thmb` first |

Default thumbnail mode: **keyframe at or before** seek time (same as ffmpeg `-ss` before `-i`). `ThumbnailOptions.Exact` remains VP8/VP9-only unless forward decode is cheap.

## Compare gate

Fixtures transcoded from `test/data/reference_1080_10s.mp4` via ffmpeg (`cmd/gomedia-compare/generate.go`). Thumbnail checks compare PSNR/SSIM and **timing** vs ffmpeg 9.0.2 with `expectations.json` `timingGate` (50% max regression tier 2).
