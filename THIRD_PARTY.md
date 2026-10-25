# Third-party code

## `github.com/Eyevinn/mp4ff` (direct dependency)

**Why:** H.264 **parameter set and slice header** parsing (`avc` package) — SPS/PPS validation (`internal/h264/sps.go`), NAL typing, and slice metadata for the intra decoder. MP4 **container** access uses **`github.com/abema/go-mp4`**, not mp4ff.

**Policy:** Do not add mp4ff for full MP4 manipulation without updating this file.

## H.264 intra decode (`internal/h264`)

**Target:** clean-room decode-only intra Main/High under `internal/h264/cleanroom`, written from ITU-T H.264 (ffmpeg/openh264 as behavioral references only). See `docs/FOLLOWUP_H264_CLEANROOM.md`.

**Current PR #9 state (not acceptable):** packages `intradec`, `cabac`, `cavlc`, `transform`, `pred`, `slice`, `reconstruct`, `h264context`, `frame`, `yuv` are **relocated Eyevinn/hi264** (~8.3k LOC), not a new implementation. There is **no** `github.com/Eyevinn/hi264` module entry, but relocated copies are still hi264 and must be deleted.
