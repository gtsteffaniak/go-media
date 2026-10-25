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

CGO_ENABLED=0 go run . -seed-dir "$HARNESS/.seed" -assets "$HARNESS/.assets" -report "$REPORT"
echo "Open test/compare/report_site/index.html in a browser (or: python3 -m http.server 8765 -d test/compare/report_site)"
