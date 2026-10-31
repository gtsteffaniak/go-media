<div align="center">

# go-media

**Pure-Go media probing, metadata, subtitles and keyframe thumbnails, with no `ffmpeg` or `ffprobe` subprocess.**

[![CI](https://github.com/gtsteffaniak/go-media/actions/workflows/ci.yml/badge.svg)](https://github.com/gtsteffaniak/go-media/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/gtsteffaniak/go-media/pkg/media.svg)](https://pkg.go.dev/github.com/gtsteffaniak/go-media/pkg/media)
[![Go 1.27+](https://img.shields.io/badge/go-1.27%2B-00ADD8?logo=go)](go.mod)
[![CGO_ENABLED=0](https://img.shields.io/badge/cgo-none-success)](#design-principles)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**[📊 Live comparison report](https://gtsteffaniak.github.io/go-media/)** · [Examples](examples/README.md) · [Capabilities](docs/SUPPORTED_CAPABILITIES.md) · [Docs](#documentation)

</div>

---

Built for go programs that value speed and efficiency as the primary concern (e.g. [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser)). The library is designed so anything thats unsupported will return typed errors so the app can fall back to ffmpeg without additional delay.

## Measured against ffmpeg on every commit

> [!TIP]
> **[📊 View the live comparison report →](https://gtsteffaniak.github.io/go-media/)**
> Interactive charts, side-by-side thumbnails and per-format timing, regenerated from the latest `main`.

| Report section | Shows |
|---|---|
| 🖼️ **Side-by-side thumbnails** | go-media and ffmpeg output for every format, with PSNR and SSIM |
| ⏱️ **Timing per build tier** | pure Go, pure Go + PGO (production), and PGO + SIMD |
| ✅ **Correctness gate** | duration, probe, tags, artwork, lyrics and subtitles must match ffprobe or the build fails |

To build the same report locally, run `make compare` (see [`test/compare/README.md`](test/compare/README.md)).

## Features

| Capability | Highlights |
|---|---|
| 🔎 **Probe** | Container, streams, codecs, dimensions, rotation, frame rate, sample rate |
| ⏱️ **Duration** | From headers, with a `Source` (`header`, `estimate`, `scan`) for cache provenance |
| 🏷️ **Tags & artwork** | ID3, MP4 `ilst`, Vorbis comments; APIC, `covr` and FLAC `PICTURE` cover art |
| 💬 **Subtitles** | SRT, VTT, ASS/SSA sidecars plus embedded MKV and MP4 text tracks, converted to WebVTT byte-identical to `ffmpeg -f webvtt` |
| 🖼️ **Thumbnails** | One keyframe decoded in pure Go, scaled and encoded to JPEG, PNG or lossless WebP |
| 🎵 **Lyrics** | Embedded lyrics and `.lrc` / `.elrc` sidecars |

### Thumbnail codecs

| Codec | Containers | Decoder |
|---|---|---|
| **H.264** Baseline, Main, High, High 10 | MP4, MOV, M4V, AVI, MPEG-TS | Clean-room intra decoder ([details](docs/H264.md)) |
| **VP9** profile 0 | WebM, MKV | [govpx](https://github.com/gtsteffaniak/govpx), bit-exact with libvpx |
| **VP8** | WebM, MKV | Pure Go |
| **MPEG-4 Part 2** (DivX, XviD) | MP4, MOV, MKV, AVI | Clean-room I-VOP decoder, bit-exact with ffmpeg ([details](docs/MPEG4.md)) |
| **MPEG-2** | MP4, MOV, MPEG-PS, MPEG-TS | I-picture decoder |
| **WMV3** (VC-1 Simple & Main) | ASF, WMV | Intra decoder, bit-exact with ffmpeg |
| **MJPEG** | MP4, MOV, MKV, AVI | Direct frame extract |
| **HEIC / HEIF**, camera **RAW** | HEIC, HEIF, AVIF, CR2, CR3, NEF, DNG… | Embedded JPEG preview |

> [!NOTE]
> Per-format detail, partial support and ffmpeg equivalents are in **[Supported capabilities](docs/SUPPORTED_CAPABILITIES.md)**.

## Quick start

```sh
go get github.com/gtsteffaniak/go-media
```

```go
import "github.com/gtsteffaniak/go-media/pkg/media"

f, _ := os.Open("clip.mp4")
st, _ := f.Stat()

info, err := media.Probe(f, st.Size())              // format, streams, tags
thumb, err := media.ThumbnailLarge(f, st.Size(), 10) // JPEG ~10% into the video

if errors.Is(err, media.ErrUnsupported) || errors.Is(err, media.ErrUnsupportedProfile) {
    // hand the file to ffmpeg instead
}
```

More recipes, including subtitles, tags, artwork, custom thumbnail sizes, caching and ffmpeg fallback, are in **[examples/README.md](examples/README.md)**.

Try it from the command line:

```sh
go run . probe    clip.mp4
go run . duration song.flac
go run . thumb clip.mkv --large thumb.jpg
```

> [!IMPORTANT]
> Build releases with the checked-in PGO profile, the fastest supported configuration:
> ```sh
> CGO_ENABLED=0 go build -pgo=default.pgo ./...
> ```
> `GOEXPERIMENT=simd` kernels are experimental and off by default. See [Profiling](docs/PROFILING.md) and [SIMD](docs/SIMD.md).

## Design principles

- **Pure Go.** `CGO_ENABLED=0`, no WASM and no bundled binaries, so it cross-compiles anywhere.
- **Bounded work.** It reads headers and sync samples and decodes one keyframe, never the whole file.
- **Fast or honest.** A format that can't stay close to ffmpeg's speed returns `ErrUnsupported` or `ErrUnsupportedProfile` rather than being slow.
- **Provenance.** Results carry a `Source`, so they can be cached next to ffprobe output.

> [!NOTE]
> **Not in scope:** transcoding, streaming, playback and full HEVC/AV1 decode. Use [go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg) for those; go-media is the fast path in front of it.

## Documentation

| Doc | What's inside |
|---|---|
| [Examples](examples/README.md) | Copy-paste recipes for every API |
| [Supported capabilities](docs/SUPPORTED_CAPABILITIES.md) | Full format and feature matrix with ffmpeg equivalents |
| [Quality & CI](docs/QUALITY.md) | Test matrix, the ffmpeg comparison gate and fuzzing |
| [Comparison harness](test/compare/README.md) | How the report is generated and how to run it locally |
| [Profiling & PGO](docs/PROFILING.md) | Build tiers, benchmarks and refreshing `default.pgo` |
| [SIMD](docs/SIMD.md) | The `GOEXPERIMENT=simd` experiment and when it's enabled |
| [H.264](docs/H264.md) · [MPEG-4](docs/MPEG4.md) | Decoder internals, colour handling and accuracy |
| [Third-party notices](THIRD_PARTY.md) | Dependencies and licenses |

## License

[MIT](LICENSE). Third-party components are listed in [THIRD_PARTY.md](THIRD_PARTY.md).
