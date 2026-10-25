# Comparison harness

Developer tooling (not part of the library API) that:

1. Downloads the pinned go-ffmpeg seed MP4 (`harness/seed.go`)
2. Generates a format matrix with ffmpeg (`harness/generate.go` → `harness/.assets/`, gitignored)
3. Runs go-media vs ffprobe/ffmpeg for duration, probe, tags, artwork, lyrics, subtitles, and thumbnails
4. Writes a static dashboard to `report_site/` (`data/report.json` + HTML)

```bash
# from repository root (pinned ffmpeg 9.0.2)
make ffmpeg
make compare-gate
```

- **Gate manifest:** `expectations.json` — must match `report_site/data/report.json` when `COMPARE_GATE=1`.
- **Thumbnail timing gate:** `expectations.json` → `timingGate` enforces ≤20% vs ffmpeg (10% target note) on MP4 MJPEG/H.264 assets listed in `enforcedAssetIds`. See `docs/PROFILING.md` for benchmarks and PGO.
- **Skip reasons:** N/A checks set `detail` in the report (tags on video, thumbnail on audio, etc.).

The harness module lives in `harness/` so `ffmpeg_test.go` in this directory can remain a lightweight `go test` smoke check against committed `testdata/`. Set `COMPARE_GATE_TEST=1` to run the full gate via `go test` in this package.
