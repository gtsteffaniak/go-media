#!/usr/bin/env bash
# Regenerate default.pgo from merged CPU profiles over representative thumbnail + decode workloads.
# Profiles match the shipped build (CGO_ENABLED=0 + PGO, see ShippedTimingGateBuild): full thumb pipeline, H.264 decode.
# Run this on the machine class the gate runs on (amd64); profiles taken on another arch are not representative.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=ffmpeg-path.sh
source "$ROOT/scripts/ffmpeg-path.sh"
FFMPEG_VERSION="${FFMPEG_VERSION:-9.0.2}"
export FFMPEG_DIR="${FFMPEG_DIR:-$HOME/ffmpeg-$FFMPEG_VERSION}"
prepend_ffmpeg_to_path || {
	echo "ffmpeg not found; install ffmpeg or run make ffmpeg on Linux" >&2
	exit 1
}
./scripts/generate-fixtures.sh

HARNESS="$ROOT/test/compare/harness"
ASSETS="$HARNESS/.assets"
if [[ ! -f "$ASSETS/video/mjpeg.mp4" ]]; then
	echo "Generating compare harness assets (gitignored under test/compare/harness/.assets)..."
	COMPARE_BIN="${COMPARE_BIN:-$HARNESS/.cache/gomedia-compare}"
	mkdir -p "$(dirname "$COMPARE_BIN")"
	(
		cd "$HARNESS"
		CGO_ENABLED=0 go build -pgo=off -o "$COMPARE_BIN" .
	)
	CGO_ENABLED=0 "$COMPARE_BIN" \
		-seed-dir "$HARNESS/.seed" \
		-assets "$HARNESS/.assets" \
		-report "$ROOT/test/compare/report_site" \
		-run=false
fi

ASSETS="$HARNESS/.assets"
export GOMEDIA_BENCH_MJPEG="${ASSETS}/video/mjpeg.mp4"
export GOMEDIA_BENCH_WEBM="${ASSETS}/video/webm_vp8.webm"

TMPDIR="${TMPDIR:-/tmp}/gomedia-pgo-$$"
mkdir -p "$TMPDIR"
PROFS=()
bench() {
	local name="$1"
	local bt="50x"
	if [[ "${2:-}" =~ ^[0-9]+x$ ]]; then
		bt="$2"
		shift
	fi
	shift
	local out="$TMPDIR/${name}.prof"
	echo "== PGO sample: $name (benchtime=$bt) =="
	GOMAXPROCS=1 CGO_ENABLED=0 GOFLAGS=-pgo=off go test -run='^$' -benchtime="$bt" -cpuprofile="$out" "$@" || return 1
	PROFS+=("$out")
}

bench thumb-mjpeg-small -bench=BenchmarkMJPEGSmall ./internal/thumb/
bench thumb-mjpeg-large -bench=BenchmarkMJPEGLarge ./internal/thumb/
bench thumb-h264-large -bench=BenchmarkMP4H264Large ./internal/thumb/
bench thumb-h264-small -bench=BenchmarkMP4H264Small ./internal/thumb/
bench thumb-h264-baseline-small -bench=BenchmarkMP4H264Baseline1080Small ./internal/thumb/
bench thumb-webm-large -bench=BenchmarkThumbnailLargeWebM ./internal/thumb/
bench thumb-h264-main -bench=BenchmarkMP4H264Main1080Large ./internal/thumb/
bench thumb-h264-high -bench=BenchmarkMP4H264High1080Large ./internal/thumb/
bench h264-baseline-decode -bench=BenchmarkBaselineThumbnailDecode ./internal/h264/

# -proto selects the output format once; the profiles are plain positional arguments.
go tool pprof -proto "${PROFS[@]}" >"$ROOT/default.pgo"

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
    "internal/thumb BenchmarkMP4H264Small (full pipeline)",
    "internal/thumb BenchmarkMP4H264Baseline1080Small (CAVLC + thumb)",
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
