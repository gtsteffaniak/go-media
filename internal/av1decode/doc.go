// Package av1decode decodes AV1 thumbnails with github.com/gen2brain/gav1d,
// a pure-Go, bit-exact port of dav1d.
//
//   - MP4/MKV/WebM keyframes use the av1 package's Decoder with film grain
//     skipped (SkipFilmGrain), one frame in flight, and a frame-size limit of
//     frame cap (DefaultFrameLongSide × DefaultFrameShortSide, 4096×2160 by default;
//     overridable per call via FrameLimits / media.ThumbnailOptions) checked against the frame
//     header before any picture memory is allocated. Each call uses a fresh
//     decoder that is flushed before returning, so no frame memory is held
//     between thumbnails. The sequence header's matrix_coefficients and
//     color_range are honored like the HEVC path (0 and 2 → BT.601, ffmpeg's
//     swscale default). 10/12-bit samples are rounded to 8 bits and
//     4:2:2/4:4:4 chroma is averaged to 4:2:0.
//   - AVIF stills and grids use the avif package through avif.DecodeColor.
//     clap/irot/imir are applied as ffmpeg's default autorotate does, the nclx
//     matrix and range are honored as in the HEVC path, and the decoded area
//     honors media.ThumbnailOptions.MaxSourcePixels clamped to MaxAVIFPixels.
//     RGB output (high bit depth or a transform)
//     is converted directly to 4:2:0 with 2×2 chroma averaging, which measured
//     closer to ffmpeg than top-left sampling.
//
// Output is 8-bit 4:2:0 planes in h264.ThumbYCbCr.
package av1decode
