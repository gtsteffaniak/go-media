# Comparison harness

Developer tooling (not part of the library API) that:

1. Downloads the pinned go-ffmpeg seed MP4 (`harness/seed.go`)
2. Generates a format matrix with ffmpeg (`harness/generate.go` → `harness/.assets/`, gitignored)
3. Runs go-media vs ffprobe/ffmpeg for duration, probe, tags, artwork, lyrics, subtitles, and thumbnails
4. Writes a static dashboard to `report_site/` (`data/report.json` + HTML)

```bash
# from repository root
bash scripts/run-comparison.sh
```

The harness module lives in `harness/` so `ffmpeg_test.go` in this directory can remain a lightweight `go test` smoke check against committed `testdata/`.
