# Comparison harness

Developer tooling (not part of the library API) that:

1. Downloads the pinned go-ffmpeg seed MP4 (`cmd/gomedia-compare/seed.go`)
2. Generates a format matrix with ffmpeg (`cmd/gomedia-compare/generate.go` → `cmd/gomedia-compare/.assets/`, gitignored)
3. Runs go-media vs ffprobe/ffmpeg for duration, probe, tags, artwork, lyrics, subtitles, and thumbnails
4. Writes a static dashboard to `report_site/` (`data/report.json` + HTML)

```bash
# from repository root — uses ffmpeg on PATH when available (e.g. Homebrew)
make compare

# CI gate (pinned ffmpeg 9.0.2 on Linux)
make ffmpeg
make compare-gate
```

- **Gate manifest:** `expectations.json` — must match `report_site/data/report.json` when `COMPARE_GATE=1`. Cases may set `"goarch": "arm64"` or `"amd64"` so timing expectations differ by platform (e.g. H.264 Baseline CAVLC).
- **Thumbnail timing gate:** every `thumbnail_large` / `thumbnail_small` case with `expect: supported` must stay within the regression cap in `expectations.json` → `timingGate.defaultMaxRegressionFraction` (currently **50%** slower than ffmpeg 9.0.2) on the **production build** (`CGO_ENABLED=0` + `-pgo=default.pgo`, report **tier 2 `purego_pgo`**). Tier 1 (no PGO) and tier 3 (+`GOEXPERIMENT=simd`) are diagnostic only. Cases marked `typed_unsupported` must return `ErrUnsupported` / `ErrUnsupportedProfile` and are never just asserted: the harness re-runs a rejected profile with the restriction lifted and labels the row `functionality:` (cannot decode or fails quality) or `performance:` (decodes but misses the gate), and fails with "enable it" if a gated profile starts to pass. See `docs/PROFILING.md`, `make pgo`, `make check-pgo`.
- **Skip reasons:** N/A checks set `detail` in the report (tags on video, thumbnail on audio, etc.).
- **Report timing columns:** The **Δ%** column and timing histogram count **thumbnail_small / thumbnail_large** only (median go write ms vs median ffmpeg, 50% gate). The summary **average latency** chart averages **every** check that records ffmpeg/ffprobe ms (duration, probe, thumbs, etc.). **duration** and **probe** are single-shot diagnostics, not gated. Duration’s ffmpeg ms uses minimal `format=duration` ffprobe; probe uses full ffprobe JSON (matches `media.Probe` on MP4). Go **duration/probe** ms are measured on an `os.File` reader (not the in-memory preload used for thumbnails).

### Thumbnail compare: ffmpeg ↔ go-media alignment

| Setting | go-media | ffmpeg (harness `ffmpegThumbnailPreset`) |
|--------|----------|----------------------------------------|
| Small size | 256×256 max (`ThumbnailPresetSmall`) | `scale=w='min(256,iw)':h='min(256,ih)':force_original_aspect_ratio=decrease` |
| Large size | 1280×720 max | same with 1280 / 720 |
| Downscale | `ThumbnailFilterArea` → area tile average | `flags=area` on first scale pass |
| Even dimensions | `fitThumbSize` rounds **both** W and H up to even | `scale=trunc(iw/2)*2:trunc(ih/2)*2` |
| Small quality | JPEG quality **76** | `-q:v` **9** (`31 - (76×29)/100`) |
| Large quality | JPEG quality **90** | `-q:v` **5** |
| H.264 seek | `SeekPercent` **10** (sync sample nearest 10% of **video track**, skipping a tiny black/flat keyframe) | `-ss` = `H264KeyframeTimeSec(10)`; `-skip_frame nokey` + `-skip_loop_filter all` |
| Other video seek | `SeekPercent` = `10%` of **format** duration | `-ss` = `format.duration × 10%` |
| H.264 decode | intra IDR, deblock off (`fast=true`) | matches skip flags above |
| Threads | n/a | `-threads 1` (timing fairness) |
| MJPEG / VP8 | container paths in `internal/thumb` | ffmpeg full decode + same scale/q |

**Not bit-identical:** go `image/jpeg` **Quality** uses Go’s quant tables; ffmpeg MJPEG **-q:v** uses libavcodec qscale. The harness maps 1–100 → 2–31 with `ffmpegQScale`; PSNR/SSIM gate visual parity, not identical coefficients. **Colors:** H.264 thumbnails decode to YCbCr; aggressive downscales (e.g. small preset) use **`scaleYCbCrArea` with per-tile RGB averaging** (same definition as full-frame `rgbaFromYCbCr` + area, without a 1080p RGBA buffer). Mild downscales (e.g. 1080p→large) use **YCbCr plane** area (`scaleYCbCrArea` matrix path). `Fast` on the small preset affects MJPEG/cover decode, not the H.264 matrix path. Repeated thumbs from one file can use `isobmff.OpenH264MP4Thumb` to cache `mp4.Probe` + avcC.

`TestThumbnailPresetMatchesMedia` and `TestFFmpegQScaleForPresets` in `cmd/gomedia-compare/` guard preset parity.

The compare binary lives in `cmd/gomedia-compare/` so `ffmpeg_test.go` in this directory can remain a lightweight `go test` smoke check against committed `testdata/`. Set `COMPARE_GATE_TEST=1` to run the full gate via `go test` in this package.
