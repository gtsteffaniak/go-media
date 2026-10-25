#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/testdata"
mkdir -p "$OUT"

# Short silent WAV (1s, 44100 Hz mono)
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i anullsrc=r=44100:cl=mono -t 1 \
  "$OUT/silence.wav"

# FLAC with tags
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i sine=f=440:duration=1 \
  -metadata title="Test Tone" -metadata artist="go-media" \
  -c:a flac "$OUT/tone.flac"

# MP3 CBR with ID3
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i sine=f=880:duration=2 \
  -metadata title="MP3 Test" -metadata artist="go-media" \
  -id3v2_version 3 -c:a libmp3lame -b:a 128k "$OUT/tone.mp3"

# VP8 WebM (single keyframe-ish short clip)
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i color=c=blue:s=64x64:d=1 \
  -c:v libvpx -g 1 -keyint_min 1 -frames:v 5 \
  -an "$OUT/blue.webm"

# Minimal MP4 (H.264 baseline, no audio) for probe duration
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i color=c=red:s=64x64:d=1 \
  -c:v libx264 -pix_fmt yuv420p -profile:v baseline -g 1 \
  -an "$OUT/red.mp4"

# MJPEG in MP4 for thumbnail path
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i color=c=green:s=48x48:d=0.1 \
  -c:v mjpeg -q:v 2 -frames:v 1 \
  -an "$OUT/frame.mjpeg.mp4"

echo "Fixtures written to $OUT"
