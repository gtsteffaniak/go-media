// Package hevcdecode decodes HEVC (H.265) IRAP/IDR keyframes for thumbnails.
//
// Decoding uses [github.com/gen2brain/h265/hevc] (pure Go, MIT). Thumbnail
// output follows the sequence VUI / nclx colour tags the same way H.264 and
// VP9 paths match ffmpeg's swscale defaults (see PictureThumbYCbCr).
package hevcdecode
