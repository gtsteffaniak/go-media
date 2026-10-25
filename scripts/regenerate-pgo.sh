#!/usr/bin/env bash
# Regenerate default.pgo from the MP4 thumbnail benchmark corpus (MJPEG + H.264 in MP4).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PATH="${HOME}/ffmpeg-9.0.2:${PATH}"
./scripts/generate-fixtures.sh
PROF=/tmp/gomedia-mp4-thumb.pprof
rm -f "$PROF"
GOMAXPROCS=1 CGO_ENABLED=0 go test -run='^$' \
  -bench='BenchmarkMJPEG|BenchmarkMP4H264' -benchtime=40x \
  -cpuprofile="$PROF" ./internal/thumb/
go tool pprof -proto "$PROF" > default.pgo
echo "Wrote $ROOT/default.pgo"
