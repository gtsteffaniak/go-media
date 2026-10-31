# Third-party code

## `github.com/Eyevinn/mp4ff` (direct dependency)

**Why:** H.264 **parameter set and slice header** parsing (`avc` package) — SPS/PPS validation (`internal/h264/sps.go`), NAL typing, and slice metadata for the intra decoder. MP4 **container** access uses **`github.com/abema/go-mp4`**, not mp4ff.

**Policy:** Do not add mp4ff for full MP4 manipulation without updating this file.

## H.264 intra decode (`internal/h264/cleanroom`)

Decode-only intra path for thumbnails. **No Eyevinn/hi264.** Slice/MB implementation is derived from **[mgvs/go-openh264](https://github.com/mgvs/go-openh264)** (BSD-2-Clause; ITU-T / openh264 design references) vendored under `internal/h264/cleanroom/` — see **`THIRD_PARTY_NOTICES.md`**, `NOTICE-mgvs`, and `LICENSE-mgvs-BSD-2`. Hot IDCT/recon paths call `internal/kernels/h264` (portable Go; optional `GOEXPERIMENT=simd` kernels). SPS policy and MP4 wiring remain in `internal/h264`. Regenerate `default.pgo` after decode changes (`make pgo`).

## MPEG-4 Part 2 intra decode (`internal/mpeg4`)

Written for go-media against ISO/IEC 14496-2, with ffmpeg's `mpeg4` decoder
as the behavioural reference (no ffmpeg source copied). The VLC/scan table data
comes from oxideav-mpeg4video (MIT), see **`THIRD_PARTY_NOTICES.md`**.

## MPEG-2 Video intra decode (`internal/mpeg2video`)

Adapted from **[32bitkid/mpeg](https://github.com/32bitkid/mpeg)** (MIT) into
`internal/mpeg2video` with go-media error types and I-picture-only entry via
`DecodeKeyframe`. Transport (`internal/mpegts`) and program stream
(`internal/mpegps`) demux code comes from the same upstream, integrated for
probe, duration, and thumbnails. ffmpeg 9.0.2 is the behavioural reference for
tests.

## VP9 keyframe decode (`internal/vp9decode`)

Thumbnail VP9 decode uses **[gtsteffaniak/go-vpx](https://github.com/gtsteffaniak/go-vpx)**, a fork of [thesyncim/govpx](https://github.com/thesyncim/govpx) carrying a VP9 frame-edge entropy-context fix (BSD-3-Clause; libvpx-derived), for profile 0, 8-bit 4:2:0 packets. Non-profile-0 streams return `ErrUnsupportedProfile`.

**Why a direct dependency (not vendored):** A clean-room VP9 decoder would duplicate thousands of lines of bitstream and loop-filter code already maintained as pure Go in govpx. The module is **CGO-free**, has **no WASM**, and is the same class of dependency as abema/go-mp4 for containers — one focused library for one codec. go-media wraps it in `internal/vp9decode` (BT.601 limited-range thumb output, error mapping, superframe handling) so the public API stays stable if the backend ever changes.

## HEVC keyframe decode (`internal/hevcdecode`)

Thumbnail HEVC decode uses **[gen2brain/h265](https://github.com/gen2brain/h265)** (MIT), a pure-Go HEVC/HEIC codec with amd64/arm64/riscv64 assembly tiers (AVX2/AVX-512 on capable x86). go-media wraps it for MP4/MKV sync-sample seek and HEIF primary items when no embedded JPEG thumbnail exists. Inter prediction is decoded when required to reach an IRAP keyframe; thumbnail mode always snaps to the keyframe at or before the seek time on video. See **`docs/DECODER_PATTERNS.md`** and **`THIRD_PARTY_NOTICES.md`** (rust_h265 / oxideav-h265 lineage in upstream h265).
