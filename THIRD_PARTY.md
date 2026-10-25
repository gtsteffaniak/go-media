# Third-party code

## VP8 video decode (`github.com/gen2brain/vpx/vp8`)

Thumbnail seeking for VP8 in WebM/MKV uses the **gen2brain/vpx** VP8 decoder (libwebp-derived, BSD-style license) via `internal/vp8decode`. It decodes inter frames and golden/alt-ref reference updates; `golang.org/x/image/vp8` remains in the module graph for other image paths but is not used for Matroska VP8 thumbnails.
