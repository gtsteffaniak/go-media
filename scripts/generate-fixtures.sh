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

# Open-GOP H.264 (x264 --open-gop): keyframes after the first are non-IDR I
# pictures flagged as sync samples, which ffmpeg fast seek lands on.
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i testsrc2=s=160x96:r=10:d=6 -vf gblur=sigma=2 \
  -c:v libx264 -pix_fmt yuv420p -preset veryfast -crf 18 \
  -g 20 -keyint_min 20 -sc_threshold 0 -bf 2 -x264-params open-gop=1 \
  -an "$OUT/opengop_h264.mp4"

# Full-range BT.709-tagged H.264 for VUI colour signalling.
ffmpeg -y -hide_banner -loglevel error \
  -f lavfi -i testsrc2=s=64x64:r=1:d=1 \
  -vf scale=out_range=full:out_color_matrix=bt709,format=yuvj420p \
  -c:v libx264 -g 1 -color_range pc -colorspace bt709 \
  -color_primaries bt709 -color_trc bt709 \
  -an "$OUT/full709_h264.mp4"

echo "Fixtures written to $OUT"
