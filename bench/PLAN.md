# Keeping the performance gains

This plan describes how the gains recorded in [RESULTS.md](RESULTS.md) are kept
over time. The tooling already exists in `cmd/gomedia-bench` (`gen`, `run`,
`compare`) and in `bench/golden_test.go`. What is missing is wiring it into CI
and a written policy for when a regression blocks a merge.

## 1. Golden gate (every PR, required)

- Run `CGO_ENABLED=0 go test ./bench/ -run TestGoldenOutputs -count=1` on every
  pull request. A golden mismatch fails the build.
- A golden change is only accepted when the PR explains it and regenerates the
  file with `GOMEDIA_BENCH_UPDATE_GOLDEN=1`. The reviewer checks the diff of
  `bench/golden/<GOOS>_<GOARCH>.json` item by item.
- Goldens are tied to the fixture corpus and to ffmpeg 9.0.2. A fixture or
  ffmpeg change is a baseline event, not a silent update.

## 2. Benchmark job (nightly and on performance PRs)

- Add a CI job that runs
  `go run ./cmd/gomedia-bench run -label ci-<sha> -count 5 -benchtime 1s`
  on a fixed runner type, and uploads `bench/results/ci-<sha>/` as an artifact.
- Compare against the stored baseline with
  `go run ./cmd/gomedia-bench compare bench/results/baseline bench/results/ci-<sha>`.
  Use `-format json` for dashboards.
- Run on the same runner class every time. Results from different machines are
  not comparable; record the runner in `meta.txt`.

## 3. Regression threshold

- A benchmark regresses when `compare` reports `REGRESSED`: a Welch-significant
  change of at least 5% slower.
- A run whose spread (CV) is above 5% is `UNRELIABLE`. It cannot pass or fail
  the gate. Re-run it on a quieter runner instead.
- A PR that regresses any benchmark it does not target blocks merge. A PR that
  regresses its own target must explain the trade-off in the description.

## 4. Baseline refresh

- The baseline is `bench/results/baseline`. It is refreshed only when one of
  these happens: a new fixture is added, the Go toolchain or ffmpeg changes, or
  the runner class changes. Each refresh is a separate PR that records the old
  and new numbers and the reason.
- Never refresh the baseline to hide a regression. A regression is fixed or
  accepted with a written reason.
- Each accepted optimization updates the cumulative table in `RESULTS.md`
  (baseline, final, percent change, spread, status).

## 5. Ownership

- `bench/ownership.json` maps each entry to the files it may change and the
  benchmarks and goldens that verify it. An entry's PR may edit only its owned
  paths. Shared-file changes are reported and reviewed separately.

## 6. Open items

- The CI job above does not exist yet.
- The runner class for the stored baseline is not fixed. The current baseline
  was measured in a sandbox with unknown load and CPU, so its numbers are only
  a relative reference on this machine.
