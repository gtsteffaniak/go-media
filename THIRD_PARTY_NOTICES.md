# Third-party notices

## H.264 intra decode (`github.com/Eyevinn/hi264`)

`internal/h264` currently orchestrates a **legacy vendored Eyevinn hi264** decode path (MIT, temporary) and validates SPS via **`github.com/Eyevinn/mp4ff/avc`** (MIT). A clean-room replacement is planned in `docs/FOLLOWUP_H264_CLEANROOM.md`. Behavioral references include FFmpeg’s `h264` decoder and openh264; no GPL/LGPL code is copied.

## VP8 video decode (`github.com/gen2brain/vpx/vp8`)

Thumbnail seeking for VP8 in WebM/MKV uses the **gen2brain/vpx** VP8 decoder (libwebp-derived, BSD-style license) via `internal/vp8decode`.
