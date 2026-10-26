# Thumbnail performance profiling

End-to-end thumbnail latency is gated in `test/compare` against ffmpeg 9.0.2 (see `make compare-gate`). For stage-level breakdown use the harness bench helpers and Go's profiler.

## Shipped build vs report tiers

| Variant | Used for |
|---------|----------|
| **In-process `purego_pgo_in_process`** | **≤20% support gate** (comparison harness built with `-tags purego -pgo=default.pgo`) |
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

## Stage breakdown (reference profile)

Collected on **darwin/arm64** (Apple laptop), `GOMAXPROCS=1`, `-tags purego -pgo=off`, 30 iterations of `internal/thumb` benchmarks on the harness assets (Big Buck Bunny seed, 1080p libx264 crf 18 `-g 30`, seek 10%, `large` preset). The gate machine is amd64, so use this for the **shape** of the time split, not for absolute numbers or gate decisions. Percentages are cumulative pprof shares of the whole `Generate` call (they overlap where one stage calls another).

| Stage | Main 1080 (103 ms/op) | High 1080 (107 ms/op) |
|-------|----------------------|-----------------------|
| CABAC residual parse (`residualBlockCABAC`) | 39% | 39% |
| of which `cabacEngine.decodeDecision` | 33% | 34% |
| `decodeBypass` | 3.5% | 4% |
| Chroma reconstruct (dequant + IDCT + add) | 10% | 15% |
| 4×4 dequant + IDCT + add + intra4x4 (luma) | ~12% | ~3% |
| 8×8 dequant + IDCT + intra8x8 (luma) | n/a | ~10% |
| YCbCr area downscale | 4% | 3% |
| JPEG encode (`image/jpeg.Encode`) | 12% | 12% |
| File I/O (`ioat.ReadAt` → `os.File.ReadAt` syscalls) | 10% | 10% |

MJPEG (`mjpeg.mp4`, 960 px wide, same flags): large 15.3 ms/op, small 9.5 ms/op, but each run is under 0.7 s of samples, so treat it as low confidence. The stdlib JPEG **encoder** (`writeBlock`, `div`, `yCbCrToYCbCr`) is about half of the compute on the large preset, and file `ReadAt` syscalls are 20% (large) to 54% (small). `jpegfast` `reconstructBlock` (dequant + IDCT) is only 10–19%.

Takeaways that drive the SIMD plan:

- Entropy decoding is about **55% of the H.264 decode** (39% of the whole thumbnail), and `decodeDecision` alone is a third. SIMD cannot help it; scalar bit-cache, table-driven and multi-bit-renorm work comes first.
- Everything SIMD can touch in the decoder (dequant, IDCT, intra prediction, add/clip) is about 15–25% of the whole thumbnail, so a 3× kernel speedup is worth roughly 10–15%.
- `image/jpeg.Encode` (12% of H.264, about half of MJPEG compute) is larger than the YCbCr scale (3–4%). File I/O (10%) turned out to be 28,611 `ReadAt` calls per thumbnail, 28,610 of them 64 bytes or less, from unbuffered box parsing. `ioat.Reader.Read` now has a 32 KiB read-ahead window, which cut that to 24 calls and the syscall share from 10.2% to 2.4%.

### Measured changes (same machine and method; laptop noise is about ±4%)

| Change | Main 1080 | High 1080 | Notes |
|--------|-----------|-----------|-------|
| Baseline above | ~103 ms | ~107 ms | |
| `ioat.Reader.Read` 32 KiB read-ahead | ~100 ms | ~104 ms | 28,611 → 24 `ReadAt` calls; syscall share 10.2% → 2.4% |
| CABAC engine: scaled offset, CLZ renorm, 32-bit refill | ~82 ms | ~87 ms | `decodeDecision` ~31 → ~24 ms/op; decoded YUV sha256 identical on all fixtures; differential test and fuzz vs the original engine (`cabac_ref_test.go`) |
| Branch-free `decodeDecision` (rejected) | ~89 ms | ~94 ms | Bit-exact but ~8% slower on arm64. Not measured on amd64, so re-test there before revisiting |

`decodeDecision` now costs about 3.8 ns per bin (~11 cycles), close to the dependency-chain floor of a serial arithmetic decoder, so further scalar tuning of it has little left. The remaining levers are structural (overlapping parse with reconstruction, the JPEG encoder).

To reproduce: `bash scripts/generate-fixtures.sh` plus the harness asset commands in `test/compare/harness/generate.go`, then `GOMAXPROCS=1 CGO_ENABLED=0 go test -tags purego -pgo=off -run='^$' -bench='BenchmarkMP4H264Main1080Large$' -benchtime=30x -cpuprofile=/tmp/p.prof -o /tmp/thumb.test ./internal/thumb/` and `go tool pprof -top -cum /tmp/thumb.test /tmp/p.prof`. MJPEG needs `GOMEDIA_BENCH_MJPEG=<path to mjpeg.mp4>`.

## SIMD experiment (Go 1.27+, CI on stable)

`simd` / `simd/archsimd` require `GOEXPERIMENT=simd` at build time and are **not** stable APIs. Production paths use portable Go in `internal/kernels` (the scalar amd64 asm was removed; SIMD kernels are added per kernel once they beat it). Report **tier 3** builds thumbmode with `GOEXPERIMENT=simd`. Optional experiment tests:

```bash
CGO_ENABLED=0 GOEXPERIMENT=simd go test ./internal/kernels/...
```
