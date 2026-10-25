# Third-party code

## `github.com/Eyevinn/mp4ff` (direct dependency)

**Why:** H.264 **parameter set and slice header** parsing (`avc` package) — SPS/PPS validation (`internal/h264/sps.go`), NAL typing, and slice metadata for the intra decoder. MP4 **container** access uses **`github.com/abema/go-mp4`**, not mp4ff.

**Policy:** Do not add mp4ff for full MP4 manipulation without updating this file.

## H.264 intra decode (`internal/h264`)

Implemented **in-tree** (ITU-T H.264 intra slice decode for thumbnails): `intradec`, `cabac`, `cavlc`, `transform`, `pred`, `slice`, `reconstruct` (amd64 asm + `purego`), `frame`, `yuv`. Behavioral validation uses ffmpeg/ffprobe on the comparison corpus. **No `Eyevinn/hi264` module dependency.**
