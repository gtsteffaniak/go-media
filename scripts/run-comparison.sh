#!/usr/bin/env bash
# Fetch seed media, generate fixtures, run go-media vs ffmpeg comparisons, and write report_site/.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HARNESS="$ROOT/test/compare/harness"
REPORT="$ROOT/test/compare/report_site"
cd "$HARNESS"

if ! command -v ffmpeg >/dev/null 2>&1; then
	echo "ffmpeg not found; run scripts/install-ffmpeg.sh or install ffmpeg" >&2
	exit 1
fi

EXPECTATIONS="${EXPECTATIONS:-$ROOT/test/compare/expectations.json}"
GATE_ARGS=()
if [[ "${COMPARE_GATE:-}" == "1" ]]; then
	GATE_ARGS=(-gate -expectations "$EXPECTATIONS")
fi

PGO="$ROOT/default.pgo"
COMPARE_BIN="${COMPARE_BIN:-$HARNESS/.cache/gomedia-compare}"
mkdir -p "$(dirname "$COMPARE_BIN")"
# PGO must be applied at link time (go run often skips profile-guided opts on dependencies).
CGO_ENABLED=0 go build -pgo="$PGO" -o "$COMPARE_BIN" .
# In-process gate uses amd64 asm kernels + PGO (see ShippedTimingGateBuild). Tier 3 simd
# binaries are built separately in buildThumbModeBinaries; do not set GOEXPERIMENT here.
CGO_ENABLED=0 "$COMPARE_BIN" -seed-dir "$HARNESS/.seed" -assets "$HARNESS/.assets" -report "$REPORT" "${GATE_ARGS[@]}"
PORT="${COMPARE_SERVER_PORT:-8765}"
echo "View the report: make server  →  http://localhost:${PORT}/"
