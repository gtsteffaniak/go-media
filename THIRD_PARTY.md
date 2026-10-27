# Third-party code

## `github.com/Eyevinn/mp4ff` (direct dependency)

**Why:** H.264 **parameter set and slice header** parsing (`avc` package) — SPS/PPS validation (`internal/h264/sps.go`), NAL typing, and slice metadata for the intra decoder. MP4 **container** access uses **`github.com/abema/go-mp4`**, not mp4ff.

**Policy:** Do not add mp4ff for full MP4 manipulation without updating this file.

## H.264 intra decode (`internal/h264/cleanroom`)

Decode-only intra path for thumbnails. **No Eyevinn/hi264.** Slice/MB implementation is derived from **[mgvs/go-openh264](https://github.com/mgvs/go-openh264)** (BSD-2-Clause; ITU-T / openh264 design references) vendored under `internal/h264/cleanroom/` — see **`THIRD_PARTY_NOTICES.md`**, `NOTICE-mgvs`, and `LICENSE-mgvs-BSD-2`. Hot IDCT/recon paths call `internal/kernels/h264` (portable Go; optional `GOEXPERIMENT=simd` kernels). SPS policy and MP4 wiring remain in `internal/h264`. Regenerate `default.pgo` after decode changes (`make pgo`).
