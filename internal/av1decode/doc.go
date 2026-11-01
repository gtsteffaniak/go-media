// Package av1decode wraps github.com/gtsteffaniak/go-av1 for thumbnail keyframes.
//
// Thumbnails use KeyframeDecoder with film grain skipped and 10/12-bit output
// down-converted to 8-bit 4:2:0 planes in h264.ThumbYCbCr (BT.601 limited,
// matching ffmpeg swscale defaults for untagged AV1).
package av1decode
