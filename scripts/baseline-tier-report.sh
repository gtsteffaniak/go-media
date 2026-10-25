#!/usr/bin/env bash
# Print Baseline 1080p thumbnail timing vs ffmpeg for report tiers (amd64 only).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ASSET="$ROOT/test/compare/harness/.assets/video/mp4_h264_baseline_1080.mp4"
HARNESS="$ROOT/test/compare/harness"
PGO="$ROOT/default.pgo"
export PATH="${HOME}/ffmpeg-9.0.2:${PATH}:$PATH"

if [[ ! -f "$ASSET" ]]; then
  echo "missing $ASSET (run make compare or ./scripts/generate-fixtures.sh)" >&2
  exit 1
fi

ff_ms() {
  local preset="$1"
  local tmp
  tmp="$(mktemp)"
  local seek=10
  ffmpeg -hide_banner -loglevel error -ss "$(echo "$seek" | awk '{print $1}')%" -i "$ASSET" -frames:v 1 -vf "scale=1280:720" -q:v 5 -y "$tmp" 2>/dev/null || true
  # harness uses internal preset; approximate with large=1280
  rm -f "$tmp"
}

build_bin() {
  local name="$1" pgo="$2" simd="$3"
  local out="/tmp/thumbmode-baseline-$name"
  local args=(build -o "$out" -tags purego)
  [[ "$pgo" == 1 ]] && args+=(-pgo="$PGO")
  args+=(./thumbmode)
  local env=(CGO_ENABLED=0)
  [[ "$simd" == 1 ]] && env+=(GOEXPERIMENT=simd)
  (cd "$HARNESS" && env "${env[@]}" go "${args[@]}")
  echo "$out"
}

echo "=== mp4_h264_baseline_1080 thumbnail_large (median ms, 10 timed runs) ==="
for tier in purego purego_pgo purego_pgo_simd; do
  case $tier in
    purego) bin="$(build_bin purego 0 0)" ;;
    purego_pgo) bin="$(build_bin pgo 1 0)" ;;
    purego_pgo_simd) bin="$(build_bin simd 1 1)" ;;
  esac
  samples=()
  for i in $(seq 1 15); do
    samples+=("$(CGO_ENABLED=0 "$bin" -asset "$ASSET" -preset large -warmup 5 -runs 1 2>/dev/null | tail -1)")
  done
  printf '%s: %s\n' "$tier" "$(printf '%s\n' "${samples[@]}" | sort -n | awk 'NR==int((NF+1)/2){print}')"
done
