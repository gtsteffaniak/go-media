#!/usr/bin/env bash
# Download and verify the pinned go-ffmpeg seed clip (used by the comparison harness).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEST="${1:-$ROOT/cmd/gomedia-compare/.seed}"
mkdir -p "$DEST"
URL="https://raw.githubusercontent.com/gtsteffaniak/go-ffmpeg/eaaf6b064cf7143a719af6c50e222914b525e88b/test/data/Big_Buck_Bunny_1080_10s_2MB.mp4"
OUT="$DEST/Big_Buck_Bunny_1080_10s_2MB.mp4"
WANT="bad8caf78fbf9ca292db2f170f99ff6251782947c0bfa8bfd391b4da4a459163"
curl -fsSL --retry 3 "$URL" -o "$OUT"
echo "$WANT  $OUT" | sha256sum -c -
echo "Seed OK: $OUT"
