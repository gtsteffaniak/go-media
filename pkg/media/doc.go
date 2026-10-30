// Package media provides pure-Go fast paths for probing, metadata, subtitles,
// and thumbnails used by file browsing applications. Unsupported inputs return
// typed errors so callers can fall back to ffmpeg.
//
// # Thumbnail parity with ffmpeg
//
// Thumbnails aim to look like ffmpeg's keyframe capture (-ss before -i,
// scale, then mjpeg/libwebp/png):
//   - Seeking lands on an MP4 sync sample (stss) and decodes it alone. Open-GOP
//     keyframes (non-IDR sync samples) are decoded as a fresh start when every
//     slice is intra or a recovery point SEI has recovery_frame_cnt 0; anything
//     else returns ErrUnsupportedProfile.
//   - H.264 colour honors the SPS VUI range and matrix tags (the MP4 colr box
//     is not read). Untagged video is BT.601 limited range at every
//     resolution, swscale's default.
//   - JPEG and WebP output are BT.601 for every source, since ffmpeg's capture
//     stays in YUV and drops the matrix tag; PNG output keeps the tag.
//   - VP8 is BT.601 limited range.
//
// Residual differences: full-range and BT.709 sources lose about 5 dB PSNR to
// RGB clipping (ffmpeg stays in YUV), and JPEG quantization differs from
// ffmpeg's mjpeg encoder.
//
// # Subtitle parity with ffmpeg
//
// SubtitleToWebVTT and ExtractEmbeddedSubtitle port ffmpeg's subtitle
// pipeline (demuxer, text decoder to ASS, webvtt encoder and muxer), so their
// output is byte-identical to "ffmpeg -i in -map 0:s:0 -f webvtt out.vtt" for
// SRT, WebVTT, ASS and SSA sidecars, Matroska S_TEXT/UTF8, S_TEXT/ASCII,
// ASS/SSA and D_WEBVTT tracks, and MP4 tx3g. Golden tests pin this against
// ffmpeg 7.1.5. Matroska S_TEXT/WEBVTT and MP4 wvtt, which ffmpeg cannot
// read, go through the same encoder. Bitmap formats (PGS, VobSub, DVB),
// HDMV TEXTST and ARIB captions return ErrUnsupported naming the format.
//
// Intentional differences: a malformed D_WEBVTT block drops only that cue
// (ffmpeg loses the rest of its cluster), unterminated laces do not run into
// the next lace, MP4 edit lists with a non-zero media_time are ignored, and
// SRT or ASS input with no cues is an error. See
// docs/SUPPORTED_CAPABILITIES.md.
//
// Import: github.com/gtsteffaniak/go-media/pkg/media
package media
