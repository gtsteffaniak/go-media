# Third-party notices

## H.264 intra decode (`github.com/Eyevinn/hi264`)

`internal/h264` uses the **Eyevinn hi264** IDR decoder (MIT) for CAVLC/CABAC intra slice decode, with SPS validation via **Eyevinn mp4ff** (`avc` package). Structural and behavioral references include [h264bsd](https://github.com/oneam/h264bsd) and FFmpeg’s `h264` decoder; no LGPL code is copied.

## VP8 video decode (`github.com/gen2brain/vpx/vp8`)

Thumbnail seeking for VP8 in WebM/MKV uses the **gen2brain/vpx** VP8 decoder (libwebp-derived, BSD-style license) via `internal/vp8decode`.
