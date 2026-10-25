# go-media vs ffmpeg comparison

This document summarizes the **local comparison harness** that exercises go-media APIs against ffmpeg/ffprobe on a shared seed corpus. The full interactive report is generated under `test/compare/report_site/` (see [Running the suite](#running-the-suite)).

## Seed media

We use the same reference clip as [go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg) (`Big_Buck_Bunny_1080_10s_2MB.mp4`), but fetch it with a **pinned raw GitHub URL + SHA-256 verification** instead of a git submodule. That keeps clones lightweight (no go-ffmpeg history or LFS) while remaining reproducible.

| Field | Value |
|-------|--------|
| Upstream commit | `eaaf6b064cf7143a719af6c50e222914b525e88b` |
| SHA-256 | `bad8caf78fbf9ca292db2f170f99ff6251782947c0bfa8bfd391b4da4a459163` |

Implementation: `test/compare/harness/seed.go`.

## Headline results (pinned ffmpeg 9.0.2)

| Metric | Count |
|--------|------:|
| Assets | 17 |
| API checks | 119 |
| **Supported** (matches ffmpeg within tolerance) | 47 |
| Typed unsupported (`ErrUnsupported` / profile) | 8 |
| Mismatch | 0 |
| Error | 0 |
| Skipped (not applicable) | 64 |

### Why 64 checks are skipped

Skips are **expected N/A rows**, not failures. Each asset runs seven checks (`duration`, `probe`, `tags`, `artwork`, `lyrics`, `subtitles`, `thumbnail`). A check is skipped when it does not apply to that fixture—for example `tags` on video-only files, `thumbnail` on audio, `lyrics` without an `.lrc` sidecar, or `subtitles` when the asset is not a subtitle-focused case. The CLI summary and `report.json` `detail` field document the reason per row.

**Takeaways**

- **Duration + probe** are strong on MP4/MOV/MKV/WebM, WAV/FLAC/OGG/MP3/M4A fixtures derived from the seed or synthetic audio.
- **Tags + artwork** align for ID3, Vorbis, and MP4 `ilst` on the generated audio matrix; FLAC embedded pictures are detected.
- **Subtitles**: sidecar SRT/VTT/ASS convert to WebVTT; MKV embedded SRT extracts; MP4 embedded `tx3g` + `stpp` (ffmpeg-generated) list and extract; `wvtt` parsing is unit-tested (pinned ffmpeg cannot mux WebVTT into ISOBMFF).
- **Thumbnails**: MJPEG-in-MP4 uses the sample table; VP8 in WebM/MKV seeks via cluster blocks and decodes inter frames (golden/alt-ref) with PSNR ~27.6 / SSIM 1.0 vs ffmpeg at 10% seek. H.264 in MP4/MOV/MKV remains typed-unsupported until the intra decoder lands.
- **Artwork**: cover detection uses `attached_pic` disposition (not full-motion MJPEG video).

## Report dashboard

![Comparison report dashboard](comparison/report-dashboard.png)

Interactive charts (timing histograms, capability breakdown, per-asset latency) are rendered with vendored Chart.js in the report site:

![Comparison report charts](comparison/report-charts.png)

## Thumbnail comparisons

### MJPEG (supported)

![MJPEG thumbnail: ffmpeg vs go-media](comparison/mjpeg-thumbnail-side-by-side.png)

### VP8 WebM (supported)

![VP8 WebM thumbnail: ffmpeg vs go-media](comparison/vp8-thumbnail-side-by-side.png)

### VP8 MKV (supported)

![VP8 MKV thumbnail: ffmpeg vs go-media](comparison/mkv-vp8-thumbnail-side-by-side.png)

## Running the suite

Requirements: Go 1.26+, **ffmpeg/ffprobe 9.0.2** on `PATH` (install with `make ffmpeg` or `bash scripts/install-ffmpeg.sh`). Distro ffmpeg versions can change generated fixtures and break the gate.

```bash
make compare          # report only
make compare-gate     # report + expectations.json gate (CI uses this)
make compare-docs     # compare + Playwright screenshots for docs/comparison/
# open test/compare/report_site/index.html
```

Doc screenshots (`docs/comparison/*.png`) are **browser captures** of the rendered report (`test/compare/report_site/index.html?capture=1`), not composited images. The MJPEG and VP8 shots use the report’s clip-path compare slider (ffmpeg / go-media labels). Regenerate with `make compare-docs` (requires Node + `npm install` under `scripts/` on first run).

`report.json` includes per-row `timingDeltaPct` ((go-media − ffmpeg) / ffmpeg), aggregate timing stats, capability breakdown, and optional regression flags when a previous `data/report.json` is present on disk before the run.

Manifest: [`test/compare/expectations.json`](../test/compare/expectations.json) lists the allowed status for all 119 `(asset, check)` pairs. `COMPARE_GATE=1` or `-gate` fails on any drift.

Regenerate only comparisons (reuse fixtures):

```bash
cd test/compare/harness
CGO_ENABLED=0 go run . -generate=false -fetch-seed=false
```

CI **Comparison** and **compare-gate** jobs run `make compare-gate` and upload `test/compare/report_site/` as an artifact on every `main` push and pull request.

## When to use go-ffmpeg instead

For production file browsing, transcoding, HLS, hardware encode, and formats outside the pure-Go matrix, use **[go-ffmpeg](https://github.com/gtsteffaniak/go-ffmpeg)**. go-media is intentionally a **fast path** with typed fallback — not a libav replacement.
