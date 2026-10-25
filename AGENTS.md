# Agent guide (go-media)

Instructions for autonomous coding agents working in this repository.

## Purpose and positioning

- **go-media** is a pure-Go breakout of **core media functionality** for apps like [FileBrowser Quantum](https://github.com/gtsteffaniak/filebrowser): probe, duration, tags, subtitles, artwork, and **server-side decoded thumbnails** (JPEG/WebP), using `io.ReaderAt` and minimal I/O.
- It is **not** a general **ffmpeg replacement** and **not** decode-only: encoding may be added later where it stays pure Go and in scope.
- For transcoding, HLS, hardware decode, exotic containers, and full libav capability, use **[gtsteffaniak/go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg)**. go-media is the **fast path**; on unsupported input return typed errors and let the caller exec ffmpeg.

## Layout

| Path | Role |
|------|------|
| **Repo root (`package media`)** | Public API: `Probe`, `Duration`, tags, subtitles, `Thumbnail*` / presets, `H264KeyframeYUV420`, errors (`ErrUnsupported`, `ErrUnsupportedProfile`). |
| `internal/sniff`, `internal/format`, `internal/detect` | Format detection and IDs. |
| `internal/isobmff` | MP4/MOV: boxes, sync samples, MJPEG/H.264 thumbnail demux (`abema/go-mp4`). |
| `internal/matroska` | MKV/WebM: cues, VP8/MJPEG thumbnails. |
| `internal/h264` | In-tree intra-only H.264 decode (`intradec`, `cabac`, `reconstruct`, …). Main/High supported; Baseline rejected. **Do not add `Eyevinn/hi264`.** |
| `internal/vp8decode` | VP8 picture → RGBA/YCbCr scale helpers. |
| `internal/jpegfast` | Fast MJPEG decode for thumbnails. |
| `internal/thumb` | Thumbnail pipeline: seek, decode, YCbCr/RGBA scale, JPEG/PNG/WebP encode. |
| `internal/kernels` | Hot loops: YCbCr area downscale, reconstruct helpers; **amd64 asm** + **`purego`** + optional **`GOEXPERIMENT=simd`** (bit-identical tests). |
| `internal/tagread`, `internal/mp3`, `internal/flac`, `internal/ogg`, `internal/wav` | Tags and duration helpers per format. |
| `internal/subtitle` | Sidecar and MKV subtitle parse/convert. |
| `internal/errs` | Typed unsupported/profile/corrupt errors. |
| `test/compare/harness` | ffmpeg 9.0.2 comparison harness, timing gate, build-mode timings, HTML report. |
| `docs/` | `COMPARISON.md`, `PROFILING.md`, `FOLLOWUP_H264_CLEANROOM.md`. |
| `default.pgo` | Checked-in PGO profile for release-style builds. |

## Hard rules

1. **Pure Go only:** `CGO_ENABLED=0` for all builds, tests, and CI. **No WASM.** No spawning ffmpeg inside library code (harness/scripts only).
2. **License:** MIT. Dependencies must be compatible; vendored code **must** be documented in **`THIRD_PARTY.md`**.
3. **Go version:** **`go.mod` minimum `go 1.27`**. **CI** uses **`go-version: stable`** + **`check-latest: true`**.
4. **Do not use [Eyevinn/hi264](https://github.com/Eyevinn/hi264)** as a module or vendored tree. All H.264 decode work stays in **`internal/h264`** (ITU-T H.264; ffmpeg/openh264 as behavioral references only — no GPL/LGPL copies).
5. **Dependencies:** keep the graph small. **`github.com/Eyevinn/mp4ff`** is allowed **only where it pays for itself** (today: **`avc` SPS/PPS/slice/NAL** parsing in `internal/h264` validation and the legacy decoder path). Do not add mp4ff for MP4 container work — use **`abema/go-mp4`** for boxes/demux.
6. **SIMD:** Design hot paths for **Go simd / `GOEXPERIMENT=simd`** where practical, alongside **stable amd64 asm** and **`-tags purego`** fallbacks — all **bit-identical** on shared tests. The comparison report records **default / purego / simd** thumbnail timings vs ffmpeg; **the ≤20% gate uses default (in-process) only**.
7. **Performance policy:** Supported features ≤**20%** slower than **ffmpeg 9.0.2** exec (10% target). Failures → **`ErrUnsupported`** / **`ErrUnsupportedProfile`**. No timing exemptions.
8. **Quality:** PSNR/SSIM vs ffmpeg; H.264 intra decode YUV metrics on keyframe fixtures.

## Thumbnail presets (FileBrowser-aligned)

- **`small`:** 256×256 max, JPEG Q **76**, area downscale, fast preview where applicable.
- **`large`:** 1280×720 max, JPEG Q **90**, area downscale.

## How to test

```bash
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go test -tags purego ./...
make compare-gate
make pgo && make profile && make bench-thumb
```

Report: `test/compare/report_site/` includes **build mode** charts (`buildModeTimings` in `report.json`).

## CI and PR workflow

- Triggers: **`push` to `main`**, **`pull_request`**. No cursor-branded workflow names.
- Matrix: amd64/arm64, default/`purego`, **Go stable**, compare-gate (PGO check), **`GOEXPERIMENT=simd`** job (kernels + report simd subprocess timings; non-blocking for merge).
- **CodeRabbit:** docstring coverage ≥ **80%** on touched public API.
- **Before finishing:** CI green; resolve all CodeRabbit threads.
