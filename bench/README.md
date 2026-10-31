# go-media benchmark framework

A repeatable benchmark system for the public API in `pkg/media`, the internal
packages it uses, and every supported container and codec in the fixture
corpus. The metric is **milliseconds per operation** (ms/op). Results are
stored as raw `go test -bench` output so any run can be re-compared later.

It is an opt-in tool for performance work, not part of CI. Nothing here runs
on pull requests: the benchmarks only run with `-bench`, and the golden check
skips unless you have generated fixtures and recorded a golden locally. The
ffmpeg comparison gate in `test/compare` remains the PR-level performance check.

Video fixtures are derived from the reference clip
`test/data/reference_1080_10s.mp4` (scaled and looped per entry), and the WMV
fixture is `test/data/reference_1080_10s_wmv3.wmv`, the same sources the
comparison harness uses.

## Layout

| Path | Purpose |
|------|---------|
| `bench/manifest.json` | Fixture corpus: one entry per input file (format, codec, flags, how to generate it). |
| `bench/manifest/` | Go package that parses and validates the manifest (shared by the generator and the benchmarks). |
| `bench/*_test.go` | Benchmarks (`public_test.go`, `micro_test.go`) and the golden output check (`golden_test.go`). |
| `bench/golden/<GOOS>_<GOARCH>.json` | Local hashes of every public output per fixture (gitignored; record on the base commit). |
| `bench/.data/` | Generated fixtures (gitignored). |
| `bench/results/<label>/` | Raw results: `meta.txt` (host, Go, git revision, flags) and one `*.txt` per suite. |
| `cmd/gomedia-bench/` | The tool: `gen`, `run`, `compare`. |

## Prerequisites

- Go (the module's toolchain), `CGO_ENABLED=0` (pure Go; the tool sets it).
- `ffmpeg` on `PATH` to generate fixtures (the comparison harness uses 9.0.2; the
  generated goldens are tied to the fixtures, so keep the same ffmpeg when you
  regenerate).

## Quick start

```bash
# 1. Generate the fixture corpus (idempotent; -force to regenerate)
go run ./cmd/gomedia-bench gen

# 2. On the base commit, record output hashes (the golden check)
GOMEDIA_BENCH_UPDATE_GOLDEN=1 CGO_ENABLED=0 go test ./bench/ -run TestGoldenOutputs -count=1

# 3. Run a labelled benchmark set: 5 repetitions, 1 s each, GOMAXPROCS=1, PGO build
go run ./cmd/gomedia-bench run -label baseline -count 5 -benchtime 1s

# 4. Compare two labels (mean ± stddev, % change, Welch t, verdict)
go run ./cmd/gomedia-bench compare bench/results/baseline bench/results/opt-probe
```

`run` must be started from the repository root. Each run writes
`bench/results/<label>/e2e.txt`, `internal.txt` and `meta.txt`.

Each job is one operation on one fixture (`Probe__mp4_h264_...`), the micro
benchmarks as a single job, or one internal package. Each writes its own shard
once, when it finishes, to `bench/results/<label>/shards/<suite>/`. A failure
therefore stays inside one operation and one fixture, and the other jobs finish
normally.

To finish a run after failures, rerun with `-retry-failed`. It reruns only jobs
whose shard is missing or failed and keeps the rest. Use `-dry-run` to list the
jobs without running them, for example to pick a subset with
`-op` or `-fixture`.
The report files are rebuilt from the ok shards at the end of every run, so
parallel runs on different fixtures can share one label and the report keeps
growing. A shard that failed is marked `status=failed` and left out of the
report. A label may not mix shards measured with different `-count`,
`-benchtime`, `-cpu`, or `-pgo`; the report refuses to merge them, so use a new
label for different settings. A lock file stops two runs from writing the same
fixture shard at once.

### Useful variants

```bash
# Only one operation across all fixtures
go run ./cmd/gomedia-bench run -label probe-try -suite e2e -op Probe -count 5

# One fixture across all operations (exact fixture name; comma-separate to pick several)
go run ./cmd/gomedia-bench run -label mp4-1080 -suite e2e -fixture mp4_h264_1920x1080_60s_large -count 5

# One operation on one fixture
go run ./cmd/gomedia-bench run -label thumb-1080 -suite e2e -op ThumbnailLarge -fixture mp4_h264_1920x1080_60s_large -count 5

# Parallel: 12 operation-and-fixture jobs at a time
go run ./cmd/gomedia-bench run -label full -suite e2e -jobs 12 -count 5

# Finish a run after some jobs failed, rerunning only those
go run ./cmd/gomedia-bench run -label full -suite e2e -jobs 12 -count 5 -retry-failed

# Internal micro benchmarks only
go run ./cmd/gomedia-bench run -label kernels -suite internal -count 10

# Regex selection of benchmarks (-op takes one exact name), via plain go test
go test -run '^$' -bench 'IDCT|Residual' -count 10 ./internal/...

# Fast smoke check (one iteration each, no statistics)
CGO_ENABLED=0 go test ./bench/ -run '^$' -bench . -benchtime 1x -cpu 1
```

Plain `go test -bench` works too; the tool only adds repetition, metadata and
the comparison.

## What is measured

- **Probe/Duration/ReadTags/HasArtwork/EmbeddedArtwork/InspectAudio/Lyrics\*/ListSubtitleTracks/
  ExtractEmbeddedSubtitle/MP4KeyframeSamples**: one sub-benchmark per applicable fixture.
- **Thumbnail\*/ThumbnailRasterPNG/H264Keyframe\***: fixtures with `"thumb": true`.
- **SubtitleToWebVTT**: sidecar text files (SRT, ASS, VTT).
- **ExternalSubtitleTracks, SIMDActive, OptionsForPreset, ...**: micro benchmarks without media.
- Inputs are loaded into memory before timing (`bytes.Reader`), so the numbers
  are CPU cost, the same way the compare harness preloads. Disk latency is not included.
- Internal packages (`internal/thumb`, `isobmff`, `matroska`, `h264`, `mpeg4`, `kernels/h264`)
  are benchmarked by the `internal` suite using their own existing benchmarks.

Operations not benchmarked directly, and why:

- `DecodeH264Keyframe` takes a raw NAL unit; the path is covered by the internal
  `BenchmarkBaselineThumbnailDecode` and the `h264` benchmarks.
- `HasLyrics` and `LyricsSidecar` are benchmarked on every media fixture, so they are covered.

## Adding a fixture (a new case)

1. Add an entry to `bench/manifest.json`. Pick the fields that match what the file is:
   - `kind`: `video`, `audio` or `subtitle` (sidecar text).
   - `container` and `codec`: used for grouping and selection (`codec == "h264"` selects the H.264 benchmarks).
   - `thumb`: true only if `Thumbnail` is supported for this file (check first; unsupported codecs use `false`).
   - `subtitles`, `artwork`: true when the file has embedded subtitle streams or cover art.
   - `corrupt`: true when errors are the expected outcome.
   - How the file is made: `ffmpeg` (argument vector; `{out}` is the output, `{dir}` the data dir, `{repo}` the repository root), `copy_from` (a repo-relative file), `truncate_of` + `truncate_bytes`, or `generate` (`srt`, `vtt`, `ass`, `lrc` text).
   - `sidecars`: extra text files next to the fixture, keyed by extension.
2. `go run ./cmd/gomedia-bench gen -only '^your_name$'` (or without `-only`).
3. Run the smoke check (`-benchtime 1x`). Any fixture that breaks a benchmark fails loudly.
4. Re-record your local golden and baseline if you are mid-way through a before/after comparison.

To add an operation, add a `Benchmark*` function in `bench/public_test.go` (or
`micro_test.go`) using `forEach` for per-fixture cases, and add its outputs to
`computeGolden` in `bench/golden_test.go`.

## Comparing runs

`compare` reads two label directories and prints:

- `Base ms` / `New ms`: mean ± sample stddev over the `-count` repetitions.
- `CV%`: coefficient of variation (above 5% is flagged `(noisy)`).
- `Δ%`: change of the mean. Negative is faster.
- `t`: Welch's t statistic. A change counts only when `|t|` exceeds the 95% critical
  value for the Welch-Satterthwaite degrees of freedom.
- `Verdict`:
  - `improved`: significant and at least 5% faster.
  - `REGRESSED`: significant and at least 5% slower.
  - `changed <threshold`: significant but under 5%.
  - `unchanged`: not significant.
- Summary: geometric mean of the new/base ratios per operation, per format, and
  overall. The cumulative change is `(geomean - 1)`.

```bash
go run ./cmd/gomedia-bench compare -format md bench/results/baseline bench/results/final
go run ./cmd/gomedia-bench compare -format text bench/results/baseline bench/results/final
```

`benchstat` gives equivalent statistics if you have it installed:
`benchstat bench/results/baseline/e2e.txt bench/results/final/e2e.txt`.

## Golden output check

`TestGoldenOutputs` runs every public operation on every fixture and compares
SHA-256 digests of the output, or of the error text, against
`golden/<GOOS>_<GOARCH>.json`. It also records each input's digest and reports
"input fixture changed" rather than comparing outputs of a different input.
Goldens are per platform (the H.264 and YCbCr kernels have architecture-specific
code) and depend on the local ffmpeg build, so they are not committed.

Workflow: record the golden on the base commit with
`GOMEDIA_BENCH_UPDATE_GOLDEN=1`, make your change, then rerun the test without
the variable. An optimization must keep it green; a change that alters output
bytes is not a pure performance change and also needs the compare harness
(`make compare-gate`).

## Reproducibility notes

- `GOMAXPROCS=1` (`-cpu 1`) and `-pgo=default.pgo` match the compare harness.
- Run on an otherwise idle machine. Do not run two benchmark sessions at once; they
  interfere with each other.
- Use `-count >= 5`. A result with CV above 5% is noisy: re-run it before drawing a conclusion.
- Always record the label, host, and git revision (`meta.txt` does this).
