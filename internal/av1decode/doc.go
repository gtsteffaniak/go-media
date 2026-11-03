// Package av1decode decodes AV1 thumbnails with two backends.
//
//   - MP4/MKV/WebM keyframes use github.com/gtsteffaniak/go-av1 KeyframeDecoder
//     with film grain skipped. It is the lower-memory choice for video: about
//     6× less memory and far fewer allocations than gav1d on the same frames.
//   - AVIF stills and grids use github.com/gen2brain/gav1d through avif.DecodeColor.
//     It is 2–4× faster than go-av1 on AVIF and uses less memory there.
//
// Output is 8-bit 4:2:0 planes in h264.ThumbYCbCr.
package av1decode
