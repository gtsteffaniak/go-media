# Thumbnail performance

[← Back to README](../README.md) · [Examples](../examples/README.md) · [Live report](https://gtsteffaniak.github.io/go-media/)

> [!TIP]
> The live report at [gtsteffaniak.github.io/go-media](https://gtsteffaniak.github.io/go-media/) compares the three build tiers side by side for each video fixture with timing data.

End-to-end latency is gated in `test/compare` against **ffmpeg 9.0.2**. Stage detail: pprof on `internal/thumb` and `internal/h264` benches. For per-operation, per-format before/after comparisons across the whole public API, use the opt-in [benchmark framework](../bench/README.md) (`cmd/gomedia-bench`), which is not run in CI.

## Production build (recommended)

Ship and benchmark with **profile-guided optimization**:

```bash
CGO_ENABLED=0 go build -pgo=default.pgo   # or: GOFLAGS=-pgo=default.pgo
```

Copy `default.pgo` from this repo into your app module (or point `-pgo` at it). Regenerate on the same **GOARCH** after hot-path changes (`make pgo`). Optional: `GOEXPERIMENT=simd` is **not** production — experiment only ([SIMD.md](SIMD.md)).

## Compare report tiers

| Tier (report id) | Role |
|------------------|------|
| **`purego_pgo` (tier 2)** | **Production** — same as above; subprocess thumbmode + **support gate** |
| `purego_pgo_in_process` | In-process harness binary for gate (`make compare-gate`) |
| `purego` (tier 1) | Diagnostic: no PGO |
| `purego_pgo_simd` (tier 3) | Diagnostic: tier 2 + `GOEXPERIMENT=simd` (not gated) |

Only **tier 2 / in-process PGO** rows define `supported` vs timing gate failure (`expectations.json`, currently 50% vs ffmpeg). Thumbnail timing uses median of 7 runs after 2 warmup iterations; gate compares **Δ%** on `go+write` vs ffmpeg (details in `test/compare/README.md`).

## Fair ffmpeg baseline

Harness ffmpeg flags align with go-media thumbs: `-skip_frame nokey`, H.264 `-skip_loop_filter all`, `-threads 1` (use `GOMAXPROCS=1` when profiling go-media).

## Commands

**Median timing (harness):**

```bash
export PATH="$HOME/ffmpeg-9.0.2:$PATH"
GOMAXPROCS=1 CGO_ENABLED=0 go test -count=1 -run='^$' -bench=ThumbnailTiming -benchtime=20x ./cmd/gomedia-compare/
```

**CPU profile (thumb pipeline):**

```bash
GOMAXPROCS=1 CGO_ENABLED=0 go test -cpuprofile=cpu.prof -run='^$' \
  -bench=BenchmarkMJPEGLarge -benchtime=30x ./internal/thumb/
go tool pprof -top -cum cpu.prof
```

**H.264 Baseline decode only:**

```bash
CGO_ENABLED=0 go test -run=^$ -bench=BaselineThumbnail -benchmem ./internal/h264/
make ceiling-profile
```

**Thumb benches (MJPEG + H.264 Main/High/WebM):** `make bench-thumb`, `make profile`.

Baseline gate policy and CAVLC vs transform split: [H264.md](H264.md).

## PGO (`default.pgo`)

Production and the compare harness use the checked-in profile at the repo root.

```bash
export PATH="$HOME/ffmpeg-9.0.2:$PATH"
make pgo          # regenerate + default.pgo.provenance.json
make check-pgo    # CI warning if hot paths changed without regen
```

Regenerate on the **gate architecture** (amd64 Linux CI). Profiles are not portable across GOARCH. Run after hot-path changes in `internal/thumb`, `internal/h264`, `internal/kernels`, demux, or `jpegfast`.

Harness sets `GOFLAGS=-pgo=default.pgo`. Thumbmode tier 1 intentionally omits PGO.

**Merged corpus** (`scripts/regenerate-pgo.sh`): MJPEG small/large; H.264 large/small (`mp4_h264`, Main/High 1080, Baseline 1080 small); WebM VP8 large; plus `BenchmarkBaselineThumbnailDecode` (decode-only CAVLC). Each workload runs **`benchtime=50x`** with `GOMAXPROCS=1` to match the compare timing gate and weight the merged profile.

H.264 thumb downscale policy (`thumbDownscaleUsesRGBArea` in `internal/thumb/ycbcr_scale.go`): aggressive ratios use **RGB tile average inside `scaleYCbCrArea`** (no full-frame RGBA prescale); mild 1080p→large presets stay on Y/Cb/Cr area + matrix.

## Reference profile shape (not gate numbers)

Example from **darwin/arm64**, PGO off, 30× `BenchmarkMP4H264*Large` on harness assets — use for **relative** stage mix, not absolute ms on amd64:

| Stage | Main 1080 | High 1080 |
|-------|-----------|-----------|
| CABAC residual + `decodeDecision` | ~39% / ~33% | similar |
| Chroma recon | ~10% | ~15% |
| Luma 4×4 / 8×8 transform | ~12% | ~13% |
| YCbCr area downscale | ~4% | ~3% |
| `image/jpeg.Encode` | ~12% | ~12% |

MJPEG large: encoder ~half of compute; `jpegfast` IDCT ~10–19%. H.264 entropy dominates Main/High; Baseline is CAVLC-heavy ([H264.md](H264.md)).

**SIMD:** optional tier 3 only — [SIMD.md](SIMD.md).
