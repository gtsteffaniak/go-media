#!/usr/bin/env bash
# Compare micro-benchmarks with benchstat (requires golang.org/x/perf/cmd/benchstat).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
COUNT="${BENCHSTAT_COUNT:-10}"
PKG="./internal/h264/ ./internal/h264/cleanroom/decode/ ./internal/kernels/h264/"
BENCH='BenchmarkBaselineThumbnailDecode$|BenchmarkResidualBlockCAVLC|BenchmarkReadLevelsInto|BenchmarkIDCTAdd16Luma'
TMP="${TMPDIR:-/tmp}/gomedia-benchstat-$$"
mkdir -p "$TMP"
echo "== benchstat -count=${COUNT} (GOMAXPROCS=1) =="
GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$' -bench="$BENCH" -count="$COUNT" $PKG >"$TMP/current.txt" 2>&1
benchstat "$TMP/current.txt"
echo "Wrote $TMP/current.txt"
