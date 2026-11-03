package av1decode

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"

	"github.com/gen2brain/gav1d/avif"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// DecodeAVIFFile decodes the primary AVIF item (still or grid) from a full file
// buffer with gav1d. gav1d is faster than go-av1 on AVIF stills (about 2–4× in
// benchmarks) and handles grids itself, so the go-av1 AVIF path is not used.
// Film grain is applied; gav1d cannot skip it.
func DecodeAVIFFile(data []byte) (*h264.ThumbYCbCr, error) {
	img, ci, err := avif.DecodeColor(bytes.NewReader(data), avif.Options{ToYCbCr: true})
	if err != nil {
		return nil, avifErr(err)
	}
	return avifToThumb(img, ci), nil
}

// avifToThumb converts a decoded AVIF image into a 4:2:0 ThumbYCbCr. Native
// YCbCr planes are used as-is and honor the file's range. Anything else is
// converted with rgbToThumb, which yields full-range samples.
func avifToThumb(img image.Image, ci avif.ColorInfo) *h264.ThumbYCbCr {
	rng := yuv.LimitedRange
	if ci.FullRange {
		rng = yuv.FullRange
	}
	switch t := img.(type) {
	case *image.YCbCr:
		if t.SubsampleRatio == image.YCbCrSubsampleRatio420 && t.Rect.Min == (image.Point{}) {
			return &h264.ThumbYCbCr{YCbCr: t, CS: yuv.BT601, RNG: rng}
		}
	case *image.NYCbCrA:
		// Alpha is dropped; thumbnails are opaque.
		if t.SubsampleRatio == image.YCbCrSubsampleRatio420 && t.Rect.Min == (image.Point{}) {
			return &h264.ThumbYCbCr{YCbCr: &t.YCbCr, CS: yuv.BT601, RNG: rng}
		}
	}
	return rgbToThumb(img)
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
	if errors.Is(err, avif.ErrUnsupported) {
		return errs.Unsup("thumbnail", "avif: "+err.Error())
	}
	return errs.UnsupProfile("avif", err.Error())
}
