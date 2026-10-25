# Third-party code

## `github.com/Eyevinn/mp4ff` (direct dependency)

**Why it is here:** H.264 **parameter set and slice header** parsing (`avc` package) — SPS/PPS validation (`internal/h264/sps.go`), NAL typing, and Annex-B / avcC sample handling on the **legacy** decode path. This is narrower than pulling mp4ff for full MP4 manipulation; container reads use **`github.com/abema/go-mp4`**.

**Policy:** Do not add mp4ff elsewhere without updating this file. Prefer extending `internal/h264` and minimal local bitstream helpers when mp4ff does not justify its weight.

## `third_party/hi264` (temporary; fork of Eyevinn/hi264 v0.10.0)

**Status: scheduled for removal.** Eyevinn/hi264 is **not** a long-term dependency. A follow-up change replaces it with a **clean-room** intra decoder in **`internal/h264`** (see `docs/FOLLOWUP_H264_CLEANROOM.md`). Until then, the fork remains wired via:

`replace github.com/Eyevinn/hi264 => ./third_party/hi264`

Patches in the fork (kernels, CAVLC LUTs, skip deblock) exist only to serve current thumbnail latency; **do not treat Eyevinn/hi264 as a reputable upstream for new work.**

## Patches in `third_party/hi264` (while it exists)

- **Transform / decode:** IDCT shortcuts, optional skip deblock for thumbnails.
- **Kernels:** amd64 Plan9 asm 4×4 luma reconstruct + `purego` fallback (bit-identical tests).
