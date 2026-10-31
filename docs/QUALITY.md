# Quality and CI

[← Back to README](../README.md) · [Examples](../examples/README.md) · [Live report](https://gtsteffaniak.github.io/go-media/)

go-media's regression safety lives in the repo: every pull request is tested on two architectures, compared against a pinned ffmpeg, and fuzzed.

> [!TIP]
> The latest results from `main` are always published at **[gtsteffaniak.github.io/go-media](https://gtsteffaniak.github.io/go-media/)**.

## At a glance

| Check | Where | What it enforces |
|---|---|---|
| 🧪 **Unit and golden tests** | `ci.yml` › `test` | `go test ./...` with `CGO_ENABLED=0` on linux/amd64 and linux/arm64, with and without `default.pgo` |
| 🔍 **Vet** | `ci.yml` › `test` | `go vet ./...` on every matrix entry |
| 🎲 **Fuzz smoke** | `ci.yml` › `test` (amd64, production) | SRT parser, H.264 SPS parser and IDR decode |
| ⚡ **SIMD smoke** | `ci.yml` | `GOEXPERIMENT=simd` kernels build and pass on the latest Go |
| 📊 **ffmpeg comparison gate** | `ci.yml` › `ffmpeg comparison gate` | Correctness and timing against ffmpeg 9.0.2 ([details](#ffmpeg-comparison-gate)) |
| 🧭 **PGO freshness** | `ci.yml` › `ffmpeg comparison gate` | `make check-pgo` warns when `default.pgo` is stale |
| 🌐 **Report publishing** | `pages.yml` | Regenerates the report on every push to `main` and publishes it to `gh-pages` |

## Test matrix

| Runner | Arch | Build | Fixtures and fuzz |
|---|---|---|---|
| `ubuntu-latest` | amd64 | production (`-pgo=default.pgo`) | ✅ |
| `ubuntu-24.04-arm` | arm64 | production (`-pgo=default.pgo`) | |
| `ubuntu-latest` | amd64 | no PGO | |
| `ubuntu-24.04-arm` | arm64 | no PGO | |

Run the same thing locally:

```sh
make test                                            # vet + go test
CGO_ENABLED=0 GOFLAGS=-pgo=default.pgo go test ./... # production build
```

## ffmpeg comparison gate

`make compare-gate` generates a corpus of real-world-style encodes (phone and camera video, screen captures, HEIC and DNG stills), runs every go-media API and the matching ffmpeg or ffprobe command on each file, and fails the build on any regression.

| Outcome | Meaning |
|---|---|
| ✅ `supported` | go-media's result must match ffmpeg or ffprobe (duration, probe, tags, artwork, lyrics, subtitles, thumbnails) |
| 🟡 `typed_unsupported` | go-media must return `ErrUnsupported` or `ErrUnsupportedProfile`, so callers fall back cleanly |
| ⏭️ `skipped` | The check doesn't apply to that asset (for example, lyrics on a video) |
| ❌ `mismatch` / `error` | Fails the gate |

Every case is declared in [`test/compare/expectations.json`](../test/compare/expectations.json).

### Thumbnail quality and timing

- **Visual parity** is measured as PSNR and SSIM against `ffmpeg -frames:v 1` on the same keyframe.
- **Timing** is gated on the production build (tier 2, `CGO_ENABLED=0` + `-pgo=default.pgo`). The enforced cap and the long-term target are `timingGate.defaultMaxRegressionFraction` and `timingGate.targetMaxRegressionFraction` in `expectations.json`.

> [!IMPORTANT]
> A format that can't keep up with ffmpeg is marked unsupported and returns a typed error, rather than being exempted from the gate.

### Run it locally

```sh
make compare        # ffmpeg on PATH, writes the report, no gate
make compare-gate   # downloads pinned ffmpeg 9.0.2 and enforces the gate
```

The report is written to `test/compare/report_site/index.html`.

> [!WARNING]
> The gate takes several minutes and is timing-sensitive. Don't run it alongside other CPU-heavy work, or the timings will be skewed.

See [`test/compare/README.md`](../test/compare/README.md) for the harness internals, alignment rules and report format.

## Fuzzing

| Target | Package | In CI |
|---|---|:---:|
| `FuzzParseSRT` | `./internal/subtitle` | ✅ |
| `FuzzParseSPS` | `./internal/h264/cleanroom/decode` | ✅ |
| `FuzzDecodeIDRAVC` | `./internal/h264/cleanroom` | ✅ |
| `FuzzParsePPS`, `FuzzCabacEngine` | `./internal/h264/cleanroom/decode` | |
| `FuzzMovieIndex` | `./internal/isobmff` | |
| `FuzzReadHead`, `FuzzClusterWalk` | `./internal/matroska` | |
| `FuzzParseRIFF` | `./internal/avi` | |
| `FuzzProbe` | `./internal/mpegts` | |
| `FuzzDecodeKeyframe` | `./internal/mpeg2video`, `./internal/mpeg4`, `./internal/vc1` | |

```sh
CGO_ENABLED=0 go test -fuzz=FuzzParseSRT -fuzztime=30s ./internal/subtitle
```

CI runs the first three briefly as a smoke test. Run the relevant target for longer locally when you touch a parser or decoder.

## Production build and PGO

Ship with the checked-in profile. It's tier 2 in the report and the build the timing gate enforces.

```sh
CGO_ENABLED=0 go build -pgo=default.pgo ./...
# or: export GOFLAGS=-pgo=$PWD/default.pgo
```

| Command | Purpose |
|---|---|
| `make pgo` | Regenerate `default.pgo` after hot-path changes |
| `make check-pgo` | Check whether the profile is stale (CI warns) |

> [!NOTE]
> `GOEXPERIMENT=simd` kernels are experimental (tier 3) and not needed for production. See [SIMD](SIMD.md) and [Profiling](PROFILING.md).
