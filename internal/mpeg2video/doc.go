// Package mpeg2video decodes MPEG-2 Video (ISO/IEC 13818-2) I-pictures for
// thumbnails. P- and B-pictures are not decoded; callers receive
// ErrUnsupportedProfile and should fall back to ffmpeg.
//
// The implementation is adapted from github.com/32bitkid/mpeg (MIT), rewritten
// for go-media error types and the thumbnail pipeline; ffmpeg 9.0.2 is the
// behavioural reference for tests. See THIRD_PARTY.md.
package mpeg2video
