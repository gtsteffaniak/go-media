package av1decode

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/gen2brain/gav1d/av1"
	"github.com/gen2brain/gav1d/avif"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// DecodeAVIFFile decodes the primary AVIF item (still or grid) from a full file
// buffer with gav1d. gav1d is faster than go-av1 on AVIF stills (about 2–4× in
// benchmarks) and handles grids itself, so the go-av1 AVIF path is not used.
// Film grain is applied; gav1d cannot skip it.
//
// The clap, irot and imir properties are applied (AutoRotate), matching
// ffmpeg, whose AVIF demuxer exports them as crop and display-matrix side data
// that the ffmpeg CLI applies by default (-autorotate). The decoded area is
// bounded by MaxAVIFPixels so a small, highly compressed file cannot request
// an unbounded allocation.
func DecodeAVIFFile(data []byte) (*h264.ThumbYCbCr, error) {
	img, ci, err := avif.DecodeColor(bytes.NewReader(data), avif.Options{
		ToYCbCr:        true,
		AutoRotate:     true,
		FrameSizeLimit: MaxAVIFPixels,
	})
	if err != nil {
		return nil, avifErr(err)
	}
	return avifToThumb(img, ci), nil
}

// MaxAVIFPixels bounds the decoded area (width × height) of an AVIF primary
// item or assembled grid, and of each grid tile. 2^27 pixels (about 134 MP)
// covers current phone and camera stills while keeping a hostile header far
// below gav1d's default of 16384×16384. Larger images return an unsupported
// error naming the limit.
const MaxAVIFPixels = 1 << 27

// avifColor maps the AVIF nclx matrix and range to the swscale matrix and
// range, the same way the HEVC path does (hevcdecode.ThumbColorFromPicture):
// unspecified (2) and 0 fall back to BT.601 like ffmpeg's swscale default,
// 1 is BT.709 and 9 is BT.2020.
func avifColor(ci avif.ColorInfo) (yuv.ColorSpace, yuv.Range) {
	cs := yuv.BT601
	if ci.Matrix != 0 && ci.Matrix != 2 {
		cs = yuv.ColorSpaceFromMatrixCoefficients(uint(ci.Matrix))
	}
	rng := yuv.LimitedRange
	if ci.FullRange {
		rng = yuv.FullRange
	}
	return cs, rng
}

// avifToThumb converts a decoded AVIF image into a 4:2:0 ThumbYCbCr. Native
// YCbCr planes keep the file's matrix and range: 4:2:0 is used as-is, and
// 4:2:2/4:4:4 chroma is subsampled to 4:2:0 without a trip through RGB, so
// the declared matrix and range still apply. Monochrome keeps its luma range
// with neutral chroma. Anything else (RGB output from AutoRotate or high bit
// depth, where gav1d already applied the file's matrix) goes through
// rgbToThumb, which yields full-range BT.601 samples.
func avifToThumb(img image.Image, ci avif.ColorInfo) *h264.ThumbYCbCr {
	cs, rng := avifColor(ci)
	switch t := img.(type) {
	case *image.YCbCr:
		return yccToThumb(t, cs, rng)
	case *image.NYCbCrA:
		// Alpha is dropped; thumbnails are opaque.
		return yccToThumb(&t.YCbCr, cs, rng)
	case *image.Gray:
		return grayToThumb(t, rng)
	}
	return rgbToThumb(img)
}

// yccToThumb returns 4:2:0 planes for src in the given matrix and range. A
// 4:2:0 image at the origin is returned as-is; other layouts are copied with
// the chroma sample nearest each 2×2 block's top-left corner.
func yccToThumb(src *image.YCbCr, cs yuv.ColorSpace, rng yuv.Range) *h264.ThumbYCbCr {
	if src.SubsampleRatio == image.YCbCrSubsampleRatio420 && src.Rect.Min == (image.Point{}) {
		return &h264.ThumbYCbCr{YCbCr: src, CS: cs, RNG: rng}
	}
	b := src.Rect
	w, h := b.Dx(), b.Dy()
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y++ {
		so := src.YOffset(b.Min.X, b.Min.Y+y)
		copy(dst.Y[y*dst.YStride:y*dst.YStride+w], src.Y[so:so+w])
	}
	for y := 0; y < h; y += 2 {
		for x := 0; x < w; x += 2 {
			si := src.COffset(b.Min.X+x, b.Min.Y+y)
			di := (y/2)*dst.CStride + x/2
			dst.Cb[di] = src.Cb[si]
			dst.Cr[di] = src.Cr[si]
		}
	}
	return &h264.ThumbYCbCr{YCbCr: dst, CS: cs, RNG: rng}
}

// grayToThumb wraps a monochrome image as 4:2:0 with neutral (128) chroma,
// keeping the luma samples and their declared range, as ffmpeg does when it
// converts gray to yuv420p.
func grayToThumb(src *image.Gray, rng yuv.Range) *h264.ThumbYCbCr {
	b := src.Rect
	w, h := b.Dx(), b.Dy()
	dst := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y++ {
		so := src.PixOffset(b.Min.X, b.Min.Y+y)
		copy(dst.Y[y*dst.YStride:y*dst.YStride+w], src.Pix[so:so+w])
	}
	for i := range dst.Cb {
		dst.Cb[i] = 128
		dst.Cr[i] = 128
	}
	return &h264.ThumbYCbCr{YCbCr: dst, CS: yuv.BT601, RNG: rng}
}

// rgbToThumb converts any image to 4:2:0 full-range BT.601 YCbCr. Chroma is
// taken from the top-left pixel of each 2×2 block. This is the slow path, used
// only for non-YCbCr or non-4:2:0 inputs.
func rgbToThumb(img image.Image) *h264.ThumbYCbCr {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	ycc := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*rgba.Stride + x*4
			yy, cb, cr := color.RGBToYCbCr(rgba.Pix[o], rgba.Pix[o+1], rgba.Pix[o+2])
			ycc.Y[y*ycc.YStride+x] = yy
			if x%2 == 0 && y%2 == 0 {
				ci := (y/2)*ycc.CStride + x/2
				ycc.Cb[ci] = cb
				ycc.Cr[ci] = cr
			}
		}
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: yuv.FullRange}
}

func avifErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, av1.ErrFrameTooLarge) {
		return errs.Unsup("thumbnail", fmt.Sprintf("avif: decoded image exceeds the %d-pixel decode limit", MaxAVIFPixels))
	}
	if errors.Is(err, avif.ErrUnsupported) {
		return errs.Unsup("thumbnail", "avif: "+err.Error())
	}
	return errs.UnsupProfile("avif", err.Error())
}
