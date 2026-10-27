#!/usr/bin/env bash
# Git bisect helper: exit 0 if Baseline decode micro-bench is within THRESHOLD of GOOD_NS.
# Usage:
#   GOOD_NS=$(go test ... -bench=BenchmarkBaselineThumbnailDecode -benchtime=5x | benchstat extract)
#   export PERF_GOOD_NS=52000000 PERF_THRESHOLD_PCT=5
#   git bisect start HEAD <good-commit>
#   git bisect run bash scripts/perf-bisect-good.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
GOOD="${PERF_GOOD_NS:-}"
THRESH="${PERF_THRESHOLD_PCT:-5}"
if [[ -z "$GOOD" ]]; then
	echo "set PERF_GOOD_NS (nanoseconds per op from good commit)" >&2
	exit 125
fi
out=$(GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$' \
	-bench=BenchmarkBaselineThumbnailDecode -benchtime=8x ./internal/h264/ 2>&1 | tail -1)
# format: BenchmarkBaselineThumbnailDecode-8   8   52345678 ns/op
ns=$(echo "$out" | awk '{print $3}')
if [[ -z "$ns" || "$ns" == *"FAIL"* ]]; then
	exit 1
fi
max=$((GOOD * (100 + THRESH) / 100))
if [[ "$ns" -le "$max" ]]; then
	exit 0
fi
echo "regressed: ${ns}ns/op > ${max}ns (good=${GOOD}, thresh=${THRESH}%)" >&2
exit 1
