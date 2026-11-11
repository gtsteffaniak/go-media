// Package av1decode decodes AV1 thumbnails with two backends.
//
//   - MP4/MKV/WebM keyframes use github.com/gtsteffaniak/go-av1 KeyframeDecoder
//     with film grain skipped. It is the lower-memory choice for video: about
//     6× less memory and far fewer allocations than gav1d on the same frames.
//     The sequence header's matrix_coefficients and color_range are honored
//     like the HEVC path (0 and 2 → BT.601, ffmpeg's swscale default).
//     DecodeSampleCached keeps a bounded pool of up to GOMAXPROCS decoders per
//     stream config, with an LRU over configs that closes only idle decoders.
//     Idle decoders are capped at one 4K decoder's frame area in total and
//     closed after 30 s unused. Sequence headers above 4K UHD (either
//     orientation) are rejected with an unsupported-profile error before a
//     decoder is built, so callers fall back to ffmpeg.
//   - AVIF stills and grids use github.com/gen2brain/gav1d through avif.DecodeColor.
//     It is 2–4× faster than go-av1 on AVIF and uses less memory there.
//     clap/irot/imir are applied as ffmpeg's default autorotate does, the nclx
//     matrix and range are honored as in the HEVC path, and the decoded area
//     is bounded by MaxAVIFPixels. RGB output (high bit depth or a transform)
//     is converted directly to 4:2:0 with 2×2 chroma averaging, which measured
//     closer to ffmpeg than top-left sampling.
//
// Output is 8-bit 4:2:0 planes in h264.ThumbYCbCr.
package av1decode
