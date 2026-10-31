// Package vp9decode decodes VP9 profile 0 key and inter frames for thumbnails.
//
// VP9 samples are treated as BT.601 limited range for JPEG output, matching
// ffmpeg's vp9 decoder on typical WebM content. Decoding uses
// github.com/thesyncim/govpx (libvpx-derived, BSD-style); see THIRD_PARTY.md.
package vp9decode
