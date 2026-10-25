#!/usr/bin/env bash
# Regenerate default.pgo from merged CPU profiles over representative thumbnail + decode workloads.
# Profiles match production paths: asm kernels (default build), full thumb pipeline, H.264 decode.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PATH="${HOME}/ffmpeg-9.0.2:${PATH}"
./scripts/generate-fixtures.sh

ASSETS="$ROOT/test/compare/harness/.assets"
export GOMEDIA_BENCH_MJPEG="${ASSETS}/video/mjpeg.mp4"
export GOMEDIA_BENCH_WEBM="${ASSETS}/video/webm_vp8.webm"

TMPDIR="${TMPDIR:-/tmp}/gomedia-pgo-$$"
mkdir -p "$TMPDIR"
PROFS=()
bench() {
	local name="$1"
	shift
	local out="$TMPDIR/${name}.prof"
	echo "== PGO sample: $name =="
	GOMAXPROCS=1 CGO_ENABLED=0 GOFLAGS=-pgo=off go test -run='^$' -benchtime=25x -cpuprofile="$out" "$@" || return 1
	PROFS+=("$out")
}

bench thumb-mjpeg-small -bench=BenchmarkMJPEGSmall ./internal/thumb/
bench thumb-mjpeg-large -bench=BenchmarkMJPEGLarge ./internal/thumb/
bench thumb-h264-large -bench=BenchmarkMP4H264Large ./internal/thumb/
bench thumb-webm-large -bench=BenchmarkThumbnailLargeWebM ./internal/thumb/
bench thumb-h264-main -bench=BenchmarkMP4H264Main1080Large ./internal/thumb/
bench thumb-h264-high -bench=BenchmarkMP4H264High1080Large ./internal/thumb/
bench h264-baseline-decode -bench=BenchmarkBaselineThumbnailDecode ./internal/h264/

MERGE_ARGS=()
for p in "${PROFS[@]}"; do
	MERGE_ARGS+=(-proto "$p")
done
go tool pprof "${MERGE_ARGS[@]}" >"$ROOT/default.pgo"

SHA=$(sha256sum "$ROOT/default.pgo" | awk '{print $1}')
HEAD=$(git rev-parse HEAD 2>/dev/null || echo unknown)
cat >"$ROOT/default.pgo.provenance.json" <<EOF
{
  "generatedAt": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "gitHead": "$HEAD",
  "profileSha256": "$SHA",
  "workloads": [
    "internal/thumb BenchmarkMJPEGSmall/Large",
    "internal/thumb BenchmarkMP4H264Large",
    "internal/thumb BenchmarkMP4H264Main1080Large",
    "internal/thumb BenchmarkMP4H264High1080Large",
    "internal/thumb BenchmarkThumbnailLargeWebM",
    "internal/h264 BenchmarkBaselineThumbnailDecode (CAVLC IDR)"
  ],
  "hotPathGlobs": [
    "internal/thumb/**/*.go",
    "internal/h264/**/*.go",
    "internal/kernels/**/*.go",
    "internal/isobmff/**/*.go",
    "internal/matroska/**/*.go",
    "internal/jpegfast/**/*.go"
  ]
}
EOF

rm -rf "$TMPDIR"
echo "Wrote $ROOT/default.pgo and default.pgo.provenance.json (sha256=$SHA)"
