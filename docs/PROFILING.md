# Thumbnail performance profiling

End-to-end thumbnail latency is gated in `test/compare` against ffmpeg 9.0.2 (see `make compare-gate`). For stage-level breakdown use the harness bench helpers and Go's profiler.

## Shipped build vs report tiers

| Variant | Used for |
|---------|----------|
| **In-process `amd64_asm_kernels_pgo_in_process`** | **≤20% support gate** (comparison harness `go run` with `GOFLAGS=-pgo=default.pgo`, default asm kernels) |
| Report tier 1 `purego` | Subprocess thumbmode: `-tags purego`, no PGO |
| Report tier 2 `purego_pgo` | Tier 1 + `-pgo=default.pgo` |
| Report tier 3 `purego_pgo_simd` | Tier 2 + `GOEXPERIMENT=simd` |

The HTML report shows **Δ% vs ffmpeg**, **ms**, and **incremental gain between tiers** for every video asset/preset. Only the **shipped in-process** row decides `supported` vs gate failure.

## Fair ffmpeg baseline

The comparison harness invokes ffmpeg with thumbnail-oriented flags aligned to go-media:

- `-skip_frame nokey` for H.264 intra paths
- `-skip_loop_filter all` for H.264 (matches go-media `SkipDeblock` fast preview)
- `-threads 1` (pair with `GOMAXPROCS=1` when profiling go-media)
- Area downscale (`flags=area` for H.264; MJPEG/VP8 presets use `fast_bilinear` or area per preset)

## Median timing (≥10 warm iterations)

```bash
export PATH="$HOME/ffmpeg-9.0.2:$PATH"
GOMAXPROCS=1 CGO_ENABLED=0 go test -count=1 -run='^$' -bench=ThumbnailTiming -benchtime=20x ./test/compare/harness/
```

`timing_bench.go` uses 9 warmup + 11 measured iterations and reports medians in the HTML report.

## CPU profile (per-stage)

```bash
export PATH="$HOME/ffmpeg-9.0.2:$PATH"
GOMAXPROCS=1 CGO_ENABLED=0 go test -cpuprofile=cpu.prof -run='^$' \
  -bench=BenchmarkMJPEGLarge -benchtime=30x ./internal/thumb/
go tool pprof -top -cum cpu.prof
```

Stages to map in pprof: demux (`isobmff`/`matroska`), entropy decode (`jpegfast`/H.264 clean-room), IDCT/kernels, YCbCr scale (`internal/kernels`, `thumb`), JPEG encode (`image/jpeg`).

### H.264 Baseline (CAVLC)

```bash
CGO_ENABLED=0 go test -run=^$ -bench=BaselineThumbnail -benchmem ./internal/h264/
```

See [`docs/BASELINE_PROFILE.md`](BASELINE_PROFILE.md) for stage notes and gate policy.

### MP4 H.264 Main/High (supported path)

```bash
make bench-thumb   # MJPEG + H.264 Main/High/WebM thumbnail benches
make profile       # same benches with -cpuprofile
```

## PGO (`default.pgo`)

Production and harness builds use **Profile-Guided Optimization** via the checked-in profile at the repo root.

### Regenerate (merged corpus)

After changing hot paths (`internal/thumb`, `internal/h264`, `internal/kernels`, demux, `jpegfast`) or adding workloads:

```bash
export PATH="$HOME/ffmpeg-9.0.2:$PATH"
make pgo
```

`scripts/regenerate-pgo.sh`:

1. Ensures comparison harness fixtures exist (`generate-fixtures.sh` + harness assets).
2. Collects CPU profiles from representative benches (MJPEG, H.264 Main/High/large, WebM VP8, Baseline CAVLC decode).
3. Merges with `go tool pprof -proto … > default.pgo`.
4. Writes **`default.pgo.provenance.json`** (`gitHead`, `profileSha256`, workload list, hot-path globs).

Regenerate **after** the clean-room H.264 rewrite so the profile matches production decode paths.

### Build flags

```bash
CGO_ENABLED=0 go build -pgo=default.pgo -o /dev/null .
```

Comparison harness (`scripts/run-comparison.sh`) sets:

```bash
export GOFLAGS="${GOFLAGS:--pgo=$ROOT/default.pgo}"
```

Thumbmode tier binaries use `-pgo=default.pgo` for tiers 2–3 only (tier 1 intentionally without PGO).

### CI staleness check

```bash
make check-pgo
```

`scripts/check-pgo-stale.sh` emits a **GitHub Actions warning** when files under provenance hot paths changed since the profile's `gitHead` without regenerating `default.pgo`.

## SIMD experiment (Go 1.27+, CI on stable)

`simd` / `simd/archsimd` require `GOEXPERIMENT=simd` at build time and are **not** stable APIs. Production paths use `internal/kernels` assembly (amd64) or portable Go (`-tags purego`). Report **tier 3** builds thumbmode with `GOEXPERIMENT=simd`. Optional experiment tests:

```bash
CGO_ENABLED=0 GOEXPERIMENT=simd go test ./internal/kernels/...
```
