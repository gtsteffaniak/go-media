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
| `internal/isobmff` | MP4/MOV: boxes, sync samples, MJPEG/H.264 thumbnail demux. |
| `internal/matroska` | MKV/WebM: cues, VP8/MJPEG thumbnails. |
| `internal/h264` | Intra-only H.264 decode for MP4 thumbnails/frames (Main/High; Baseline rejected). |
| `internal/vp8decode` | VP8 picture → RGBA/YCbCr scale helpers. |
| `internal/jpegfast` | Fast MJPEG decode for thumbnails. |
| `internal/thumb` | Thumbnail pipeline: seek, decode, YCbCr/RGBA scale, JPEG/PNG/WebP encode. |
| `internal/kernels` | Hot loops: YCbCr area downscale; **amd64 asm** + **`purego`** fallback (must match bit-for-bit). |
| `internal/tagread`, `internal/mp3`, `internal/flac`, `internal/ogg`, `internal/wav` | Tags and duration helpers per format. |
| `internal/subtitle` | Sidecar and MKV subtitle parse/convert. |
| `internal/errs` | Typed unsupported/profile/corrupt errors. |
| `third_party/hi264` | Vendored, patched H.264 decoder (`replace` in `go.mod`); see `THIRD_PARTY.md`. |
| `test/compare/harness` | ffmpeg 9.0.2 comparison harness, timing gate, HTML report. |
| `test/compare/report_site` | Generated report UI (not committed; CI uploads artifacts). |
| `docs/` | `COMPARISON.md`, `PROFILING.md`, `comparison/*.png` (Playwright captures). |
| `scripts/` | ffmpeg install, comparison runner, PGO regen, doc screenshots. |
| `default.pgo` | Checked-in PGO profile for release-style builds. |

## Hard rules

1. **Pure Go only:** `CGO_ENABLED=0` for all builds, tests, and CI. **No WASM.** No spawning ffmpeg inside library code (harness/scripts only).
2. **License:** MIT. Dependencies must be compatible; vendored or materially patched code **must** be documented in **`THIRD_PARTY.md`** (and notices as needed).
3. **Go toolchain:** **1.27.x** (match `go.mod` and `.github/workflows/ci.yml`).
4. **SIMD:** Production paths use **`internal/kernels`** Plan9 assembly on amd64 with **CPU dispatch** and a **`-tags purego`** portable implementation. Implementations **must be bit-identical** on shared tests. **`GOEXPERIMENT=simd`** is an **optional, non-blocking** experiment only—not a shipping requirement.
5. **Performance policy:** Supported features must be at most **20% slower** than an **ffmpeg 9.0.2 exec** round trip for the same operation (thumbnail: median **go+write** vs ffmpeg; **10% target**). Anything that cannot meet the gate is **unsupported**: return **`ErrUnsupported`** or **`ErrUnsupportedProfile`**. **No timing exemptions** or relaxed asset lists in `expectations.json`.
6. **Quality:** Validate against real ffmpeg/ffprobe output. Thumbnail JPEG rows use **PSNR/SSIM** vs ffmpeg; H.264 intra paths also check **decode YUV PSNR/SSIM** where the harness applies.

## Thumbnail presets (FileBrowser-aligned)

- **`small`:** fast listing thumb — **256×256 max**, JPEG Q **76**, area downscale, `Fast` preview where applicable.
- **`large`:** high-quality preview — **1280×720 max**, JPEG Q **90**, area downscale (not the small/fast path).

Output is **JPEG** by default; **PNG** / **lossless WebP** optional. Decoding happens in-process; presets are exposed as `media.ThumbnailSmall`, `media.ThumbnailLarge`, and `media.Thumbnail` with `ThumbnailOptions`.

**Supported decode paths (current):** MJPEG-in-MP4, H.264 Main/High intra in MP4/MOV, VP8 in WebM/MKV. **H.264 Baseline (CAVLC)** and **H.264 in Matroska** are unsupported for performance/API reasons.

## How to test

```bash
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
CGO_ENABLED=0 go test -tags purego ./...
make compare-gate          # pinned ffmpeg 9.0.2; COMPARE_GATE=1
make pgo                   # ./scripts/regenerate-pgo.sh → default.pgo
make bench-thumb           # stage benchmarks in internal/thumb
make profile               # benches + cpuprofile summary
```

- Fuzz smoke (CI): `go test -fuzz=FuzzParseSRT -fuzztime=10s ./internal/subtitle`
- Details: `test/compare/README.md`, `docs/PROFILING.md`

Regenerate **`default.pgo`** when thumbnail/hot-path code changes materially; CI checks `test -s default.pgo`.

## Report site and doc screenshots

1. `make compare` or `make compare-gate` → `test/compare/report_site/`
2. `make compare-docs` (or `python3 scripts/capture-comparison-docs.py`) serves the report and uses **Playwright** to write **`docs/comparison/*.png`**
3. Captures are **full-size** regions of the live report; thumbnail cards use the **ffmpeg / go-media** compare slider (labels in the report UI). Do not commit harness `.assets/` or `report_site/data/` junk—only intentional doc PNGs.

## CI and PR workflow

- **Triggers:** `push` to **`main`** and **`pull_request`** only (see `.github/workflows/ci.yml`). Do not add cursor-branded workflow names or references.
- **Matrix:** amd64 + arm64, default and **`-tags purego`**, compare-gate on ubuntu with PGO check, optional **`GOEXPERIMENT=simd`** job (`continue-on-error: true`).
- **CodeRabbit:** keep **docstring coverage ≥ 80%** on touched public API where the bot measures it.
- **Before finishing a PR:** **`make compare-gate` and CI green**; **resolve every CodeRabbit comment**, including threads opened **after your last push**; update `expectations.json` when behavior or support matrix changes.

## Agent hygiene

- Minimize diff scope; match existing style; no bench binaries (`*.test`), profiles other than **`default.pgo`**, or generated report trees in commits.
- When rejecting a codec for performance, wire **`typed_unsupported`** in `test/compare/expectations.json` and propagate **`ErrUnsupportedProfile`** through the public API.
- Read **`README.md`** support matrix before changing user-visible capability.
