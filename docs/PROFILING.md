# Thumbnail performance profiling

End-to-end thumbnail latency is gated in `test/compare` against ffmpeg 9.0.2 (see `make compare-gate`). For stage-level breakdown use the harness bench helpers and Go's profiler.

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

Stages to map in pprof: demux (`isobmff`/`matroska`), entropy decode (`jpegfast`/hi264), IDCT, YCbCr scale (`internal/kernels`, `thumb`), JPEG encode (`image/jpeg`).

### MP4 H.264 Main/High (supported path)

```bash
make bench-thumb   # BenchmarkMJPEG* + BenchmarkMP4H264Large in internal/thumb
make profile       # same benches with -cpuprofile
```

H.264 **Baseline (CAVLC)** is not profiled in go-media: decode latency is roughly 2× ffmpeg on 1080p IDR fixtures, so the API returns `ErrUnsupportedProfile` and callers use ffmpeg.

## PGO

Default profile: `default.pgo` at repo root. Regenerate after corpus changes:

```bash
./scripts/regenerate-pgo.sh
```

Build with PGO for local A/B:

```bash
CGO_ENABLED=0 go build -pgo=default.pgo -o /dev/null .
```

## SIMD experiment (Go 1.27+, CI on stable)

`simd` / `simd/archsimd` require `GOEXPERIMENT=simd` at build time and are **not** stable APIs. Production paths use `internal/kernels` assembly (amd64) or portable Go (`-tags purego`). Optional experiment tests:

```bash
CGO_ENABLED=0 GOEXPERIMENT=simd go test ./internal/kernels/...
```
