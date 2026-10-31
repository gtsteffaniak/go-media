# go-media

Pure-Go library for **probing**, **duration**, **metadata**, **subtitles**, **embedded artwork**, and **keyframe thumbnails** on common media files—without spawning `ffmpeg` or `ffprobe`.

Built for file browsers and indexers (e.g. [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)) that need fast, deterministic answers from `io.ReaderAt` with minimal I/O. Unsupported inputs return typed errors so the app can fall back to ffmpeg.

```go
import "github.com/gtsteffaniak/go-media/pkg/media"

info, err := media.Probe(r, size)
thumb, err := media.ThumbnailLarge(r, size, 10) // ~10% into the timeline
```

**Import:** `github.com/gtsteffaniak/go-media/pkg/media`  
**Demo CLI:** `go run . probe <file>` · `duration` · `thumb [--small|--large]`

## What it does today

| Area | Coverage |
|------|----------|
| **Containers** | MP4/MOV/M4A/M4V/3GP, MKV/WebM, **AVI**, **MPEG-TS/M2TS**, **MPEG-PS**, WAV, FLAC, OGG (Vorbis/Opus), MP3; **HEIC/HEIF/AVIF** stills; common **camera RAW** (embedded JPEG preview) |
| **Probe & duration** | Stream layout, codecs, tags summary; duration from headers (MP3 CBR estimate where needed; WebM when `Duration` is present) |
| **Tags & artwork** | ID3, MP4 `ilst`, Vorbis comments ([dhowden/tag](https://github.com/dhowden/tag)); APIC / `covr` / FLAC `PICTURE` |
| **Subtitles** | Sidecar SRT/VTT/ASS/SSA and embedded MKV (SRT, ASS/SSA, WebVTT) and MP4 (`tx3g`, `wvtt`) text tracks → WebVTT, byte-identical to `ffmpeg -f webvtt` (golden-tested against ffmpeg 9.0.2); bitmap tracks (PGS, VobSub, DVB) return a typed unsupported error; external track discovery |
| **Thumbnails** | MJPEG, **VP9** (WebM/MKV, profile 0 8-bit 4:2:0), VP8 (WebM/MKV), H.264 intra IDR (Baseline CAVLC + Main/High/High 10 CABAC, 8/9/10-bit 4:2:0; MP4/MOV/**AVI**/**MPEG-TS** `avc1`/`avc3`), **MPEG-2** I-pictures (MP4 `mp4v`/esds, MPEG-PS; MPEG-TS MPEG-2 demux in progress), MPEG-4 Part 2 / DivX / XviD I-VOP (MP4, MKV, **AVI**), **HEIC/HEIF embedded JPEG**, **RAW embedded JPEG**; embedded cover when `PreferEmbedded` |
| **H.264 decode** | Clean-room intra-only decoder for thumbnail keyframes ([`docs/H264.md`](docs/H264.md))—not a general player |
| **MPEG-4 Part 2 decode** | Clean-room I-VOP decoder (Simple / Advanced Simple), bit-exact with ffmpeg ([`docs/MPEG4.md`](docs/MPEG4.md)) |

Capability reference (by feature, with ffmpeg equivalents where useful): **[`docs/SUPPORTED_CAPABILITIES.md`](docs/SUPPORTED_CAPABILITIES.md)**.

**Not in scope:** transcoding, HLS, inter-frame playback, **VC-1/WMV (ASF) thumbnails** (typed unsupported until intra decoder lands), full HEVC/AV1 video decode, or full ffmpeg parity. Use **[go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg)** for that; go-media is the **fast path** in front of it.

## Quality and CI

Regression safety is built into the repo, not ad hoc manual testing.

- **`go test ./...`** with `CGO_ENABLED=0` on **linux/amd64 and linux/arm64**, with and without the checked-in PGO profile.
- **ffmpeg 9.0.2 comparison gate** (`make compare-gate`): generated corpus of **20 assets** (phone/camera-style encodes), **162** manifest cases in [`test/compare/expectations.json`](test/compare/expectations.json)—**72** must match ffmpeg/ffprobe as **supported** (duration, probe, tags, artwork, lyrics, subtitles, thumbnails where applicable). **6** cases must return typed unsupported (MKV H.264 and MKV subtitle thumbnails, WebM VP9 thumbnails). CI uploads the HTML report as an artifact.
- **Thumbnail timing:** supported thumb paths are gated on the **production build** (`CGO_ENABLED=0` + `-pgo=default.pgo`) to stay within **50%** of ffmpeg 9.0.2 wall time on the same fixture (stricter **10%** documented as the long-term target). Visual parity uses PSNR/SSIM vs `ffmpeg -frames:v 1` on keyframe decodes.
- **Fuzzing:** subtitle parsers and H.264 CABAC engine (`go test -fuzz`); CI runs a fuzz smoke on subtitles.

Run locally: `make compare` (ffmpeg on PATH) or `make compare-gate` (pinned ffmpeg). Report: `test/compare/report_site/index.html` — see [`test/compare/README.md`](test/compare/README.md).

## Design

- **Pure Go** — `CGO_ENABLED=0`, cross-compiles everywhere; no cgo, no WASM, no bundled binaries in library code.
- **Bounded scope** — read headers and sync samples; decode one keyframe for previews, not whole files.
- **Typed errors** — `ErrUnsupported`, `ErrUnsupportedProfile`, `ErrCorrupt`, `ErrNotMedia` for clean fallback.
- **Transparent provenance** — duration/thumbnail results record source (`header`, `estimate`, etc.) for caching next to ffprobe.

More: [`docs/PROFILING.md`](docs/PROFILING.md), [`docs/SIMD.md`](docs/SIMD.md).

## Repository layout

| Path | Role |
|------|------|
| [`pkg/media`](pkg/media) | Public API |
| [`internal/`](internal) | Format parsers, H.264 clean-room decoder, thumbnail pipeline |
| [`cmd/gomedia-compare`](cmd/gomedia-compare) | ffmpeg/ffprobe comparison harness |
| [`main.go`](main.go) | Small demo CLI |
| [`default.pgo`](default.pgo) | Profile for release-style builds |

## API sketch

```go
import "github.com/gtsteffaniak/go-media/pkg/media"

// Presets aligned with FileBrowser (small ≈ 256px Q76, large ≈ 1280×720 Q90)
thumb, err := media.ThumbnailSmall(r, size, 10)
thumb, err := media.ThumbnailLarge(r, size, 10)

thumb, err := media.Thumbnail(r, size, media.ThumbnailOptions{
    MaxWidth: 320, MaxHeight: 240,
    Quality:  85,
    Format:   "jpeg", // jpeg | png | webp-lossless
    SeekPercent: 10,
    Filter:   media.ThumbnailFilterArea,
})

info, err := media.Probe(r, size)
d, err := media.Duration(r, size)

tags, err := media.ReadTags(r, size)
vtt, err := media.SubtitleToWebVTT("movie.en.srt")

if errors.Is(err, media.ErrUnsupported) {
    // use go-ffmpeg / ffmpeg
}
```

See [`pkg/media/doc.go`](pkg/media/doc.go) and [`docs/SUPPORTED_CAPABILITIES.md`](docs/SUPPORTED_CAPABILITIES.md) for the full surface (`ListSubtitleTracks`, `H264KeyframeYUV420`, presets, filters, etc.).

## Production build

Ship with the checked-in PGO profile (tier 2 in compare reports—best balance of speed and portability):

```bash
CGO_ENABLED=0 go build -pgo=default.pgo ./...
# or: export GOFLAGS=-pgo=$PWD/default.pgo
```

Optional `GOEXPERIMENT=simd` kernels are experimental (tier 3); not required for production. `make pgo` regenerates the profile after hot-path changes; CI warns if it is stale (`make check-pgo`).

## H.264 thumbnails

The in-tree decoder is **intra-only** (keyframes decoded alone, like ffmpeg `-ss` fast seek): CAVLC Baseline and CABAC Main/High for 8-bit 4:2:0 plus High 10 (9/10-bit 4:2:0, bit-exact with ffmpeg before the 8-bit RGB stage), with optional fast preview (reduced deblock). Profiles that decode but fail the timing gate can be listed in `gatedProfiles` and rejected with `ErrUnsupportedProfile` so callers use ffmpeg. Implementation is clean-room MIT; ffmpeg/h264bsd are behavioral references only—no LGPL source.

**Open GOP:** a non-IDR sync sample is decoded as a fresh start when every slice is intra or a recovery point SEI has `recovery_frame_cnt` 0; anything else fails fast with `ErrUnsupportedProfile`.

**Colour (matches ffmpeg):** the SPS VUI range/matrix tags are honored (the MP4 `colr` box is not read); untagged video is BT.601 limited range at every resolution (swscale's default); JPEG/WebP output is BT.601 because ffmpeg's YUV capture drops the matrix tag, while PNG keeps it; VP8 is BT.601 limited. Known residuals: ~5 dB PSNR lower on full-range/BT.709 sources (RGB clipping) and JPEG quantizer differences.

Details: [`docs/H264.md`](docs/H264.md).

## Roadmap (selected)

- Matroska tags and MKV embedded artwork.
- AIFF/AU duration; tighter MP3 duration without estimate.
- Lossy WebP thumbnail output (research).

## License

[MIT](LICENSE)
