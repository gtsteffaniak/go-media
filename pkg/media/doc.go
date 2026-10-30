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
// Import: github.com/gtsteffaniak/go-media/pkg/media
package media
