# go-media

> **Status: initial implementation.** Core probe/duration, tags (via [dhowden/tag](https://github.com/dhowden/tag)), sidecar subtitles, MKV subtitle extract, embedded artwork, MJPEG/VP8/cover thumbnails, and an H.264 intra decoder **stub** are in place. CI runs `go vet`, `CGO_ENABLED=0` tests, and an ffprobe comparison harness.

Pure-Go media primitives: duration, metadata, subtitles, and keyframe thumbnails without ffmpeg.

## Purpose

go-media breaks out the core media functionality that can be done in **pure Go** (`CGO_ENABLED=0`, no external binaries, no WASM) so apps like [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser) can skip spawning `ffmpeg`/`ffprobe` for common cases.

**This is not an ffmpeg replacement.** go-media is a **fast path**: when it can't handle a file it returns a typed error (`ErrUnsupported`, `ErrUnsupportedProfile`, ...) and the caller falls back to ffmpeg.

### Use go-ffmpeg for full media capability

For transcoding, HLS, hardware acceleration, exotic containers, and anything outside the pure-Go matrix here, use **[gtsteffaniak/go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg)**. It wraps ffmpeg with detection, typed errors, and tests aligned to real binaries. go-media is meant to sit **in front of** go-ffmpeg: try pure Go first, fall back when you hit `ErrUnsupported` or `ErrUnsupportedProfile`.

See **[docs/COMPARISON.md](docs/COMPARISON.md)** for the ffmpeg/ffprobe comparison report, screenshots, and how to regenerate it locally or in CI.

## Design principles

- **Pure Go.** Builds with `CGO_ENABLED=0` and cross-compiles everywhere.
- **Small footprint, limited scope.** Only what file browsing, indexing and previews need.
- **Read only what's needed.** Header reads and seeks through `io.ReaderAt` instead of reading the whole file.
- **Deterministic.** The same input always gives the same output.
- **Typed errors for fallback.** Callers can always tell "unsupported" apart from "corrupt".
- **Cache-friendly results.** Callers key on path + size + mtime.
- **Reports its source.** Results record how they were obtained (header, estimate, scan) so callers can judge them and store that alongside ffprobe results.

## Validation

Every feature is checked against real `ffmpeg`/`ffprobe` output on a corpus of real files from phones, cameras and common encoders:

- **Duration:** compared with ffprobe within a defined tolerance.
- **Metadata and stream info:** field-by-field diffs against ffprobe JSON.
- **Frame decode:** compared with `ffmpeg -frames:v 1` using PSNR/SSIM thresholds.
- **Fuzzing:** all parsers and bitstream readers are fuzzed (`go test -fuzz`).
- **Published results:** run `make compare-gate` (pinned ffmpeg 9.0.2) and open `test/compare/report_site/index.html` (CI uploads the same site as an artifact). Summary: [docs/COMPARISON.md](docs/COMPARISON.md).

## Support matrix (current)

Legend: ✅ Implemented · 🟡 Partial / conditional · 📋 Planned · ❌ Out of scope (use ffmpeg)

### Duration

| Format | Method | Support |
|---|---|---|
| MP4 / MOV / M4A / M4V / 3GP | `mvhd` box | ✅ |
| MKV / WebM | Segment Info `Duration` (missing in some streamed WebM, which falls back) | 🟡 |
| WAV | data size ÷ byte rate | ✅ |
| FLAC | `STREAMINFO` total samples | ✅ |
| OGG Vorbis / Opus | last page granule position | ✅ |
| MP3 | Xing/Info/VBRI header; CBR estimate | 🟡 |
| AIFF / AU | — | 📋 |
| AAC (ADTS), MPEG-TS, AVI, FLV, WMV/ASF | — | ❌ |

### Stream info (codec, resolution, fps, sample rate, channels, bitrate)

| Format | Source | Support |
|---|---|---|
| MP4 / MOV | `stsd`, `tkhd` | ✅ |
| MKV / WebM | `Tracks` | ✅ |
| WAV / FLAC / OGG | format headers | ✅ |
| MP3 | frame header | ✅ |

### Tags / metadata

| Tag family | Formats | Support |
|---|---|---|
| ID3v1 / ID3v2 | MP3 | ✅ |
| MP4 `ilst` | M4A / MP4 | ✅ |
| Vorbis comments | FLAC / OGG | ✅ |
| Matroska `Tags` | MKV / WebM | 📋 |

The audio side may build on [dhowden/tag](https://github.com/dhowden/tag).

### Embedded artwork

| Source | Support |
|---|---|
| ID3 `APIC` | ✅ |
| MP4 `covr` | ✅ |
| FLAC `PICTURE` | ✅ |
| MKV attachments | 📋 |

### Subtitles

| Source | Support |
|---|---|
| Sidecar `.srt` / `.vtt` / `.ass`: parse and convert to WebVTT | ✅ |
| MKV text tracks (SRT, ASS/SSA, WebVTT) | ✅ |
| MP4 `tx3g` / `wvtt` | 📋 |
| MKV text tracks (extract to WebVTT) | ✅ |
| Image subtitles (PGS, VobSub) | ❌ |

### Keyframe thumbnails (decoded in pure Go, output as JPEG)

| Codec | Approach | Support |
|---|---|---|
| MJPEG | extract the frame directly | ✅ |
| VP8 (WebM/MKV) | `github.com/gen2brain/vpx/vp8` (inter + golden/alt-ref) | ✅ |
| H.264 stage 1 | intra-only IDR, CAVLC, 8-bit 4:2:0 (Baseline / Main without CABAC) | 📋 (API stub returns `ErrUnsupportedProfile`) |
| H.264 stage 2 | adds CABAC and the 8x8 transform (High profile) | 📋 |
| H.264 interlaced (PAFF/MBAFF), 10-bit, 4:2:2/4:4:4, inter prediction | — | ❌ |
| HEVC, VP9, AV1 | — | ❌ |

- **Keyframe selection:** MP4 `stss` and MKV `Cues`. Skips black frames by aiming about 10% into the video.
- **Optional fast preview mode:** reduced resolution (DC coefficients only, no deblocking) for thumbnails.

### Output

| Format | Support |
|---|---|
| JPEG (stdlib) | ✅ primary |
| PNG (stdlib) | ✅ |
| WebP lossless (pure-Go encoder) | 🟡 possible |
| WebP lossy | 🟡 research / TBD |

## H.264 keyframe decoder

A deliberately minimal, **intra-only** decoder whose only job is turning an IDR frame into an image for thumbnails. Leaving out inter prediction (motion compensation, reference frame management) removes most of a full decoder's complexity, which keeps the code small and auditable.

- **Scope:** NAL/SPS/PPS parsing, CAVLC then CABAC, intra 4x4/8x8/16x16 prediction, dequantization, 4x4/8x8 inverse transforms, deblocking (optional in preview mode).
- **Inspirations:** [h264bsd](https://github.com/oneam/h264bsd) (BSD-licensed Baseline decoder) as the structural reference. FFmpeg's `h264` decoder as a behavioral reference for High profile details. **No LGPL code is copied.**
- **Fallback:** unsupported streams return `ErrUnsupportedProfile`.

## Unexplored / potential options

These are research ideas, not commitments:

- A pure-Go **lossy WebP encoder** (VP8 intra encoding).
- **Audio waveform peaks** via pure-Go decoders (`hajimehoshi/go-mp3`, `mewkiz/flac`).
- **HEIC/AVIF embedded thumbnail** extraction (`thmb`/EXIF) without a full decode.
- **Hardware decode without cgo:** Linux V4L2 M2M ioctls through `x/sys/unix` on ARM SoCs, or runtime-loaded libva/VideoToolbox/Media Foundation through `ebitengine/purego`, as an optional feature.
- **HEVC intra-only decode** (a much larger effort).
- **Chapters** (MP4 `chpl`, MKV `Chapters`).
- **MP4 faststart / `moov` location** detection.
- **Subtitle track listing** for players.
- EPUB/office metadata. This probably belongs in a separate package.

## Non-goals

- Being a general ffmpeg/libav replacement.
- cgo, external binaries or WASM.
- Inter-frame decoding or playback.
- Damaged or truncated file recovery (ffmpeg's job).

## Intended API (draft, illustrative only)

```go
info, err := media.Probe(r, size)            // r io.ReaderAt
d, src, err := media.Duration(r, size)       // src: header | estimate | scan
img, err := media.Thumbnail(r, size, media.ThumbnailOptions{MaxWidth: 320, Fast: true})
if errors.Is(err, media.ErrUnsupported) {
    // fall back to ffmpeg
}
```

## Roadmap

1. Duration + stream info for MP4/MKV/WAV/FLAC/OGG/MP3, plus the ffprobe comparison harness.
2. Tags, embedded artwork, subtitles.
3. MJPEG + VP8 thumbnails.
4. H.264 intra stage 1 (CAVLC).
5. H.264 intra stage 2 (CABAC + 8x8).

## License

[MIT](LICENSE).
