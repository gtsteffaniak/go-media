package thumb

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/tagread"
	"golang.org/x/image/vp8"
)

type Options struct {
	MaxWidth       int
	MaxHeight      int
	SeekPercent    float64
	Format         string
	JPEGQuality    int
	Fast           bool
	PreferEmbedded bool
}

type Result struct {
	MIME   string
	Data   []byte
	Source string
	Width  int
	Height int
}

func Generate(r interface{ ReadAt([]byte, int64) (int, error) }, size int64, opt Options) (Result, error) {
	if opt.MaxWidth <= 0 {
		opt.MaxWidth = 320
	}
	if opt.SeekPercent <= 0 {
		opt.SeekPercent = 10
	}
	if opt.JPEGQuality <= 0 {
		opt.JPEGQuality = 85
	}
	if opt.Format == "" {
		opt.Format = "jpeg"
	}

	if opt.PreferEmbedded {
		if data, mime, err := tagread.Artwork(r, size); err == nil {
			return encodeImage(data, mime, opt)
		}
	}

	fmt, err := sniff.Detect(r, size)
	if err != nil {
		return Result{}, err
	}

	switch fmt {
	case format.MP4:
		if b, ok := isobmff.MJPEGFrame(r, size); ok {
			return encodeImage(b, "image/jpeg", opt)
		}
	case format.MKV, format.WebM:
		if b, ok := matroska.VP8Keyframe(r, size, opt.SeekPercent); ok {
			return decodeVP8(b, opt)
		}
	case format.MP3, format.FLAC, format.OGG:
		if data, mime, err := tagread.Artwork(r, size); err == nil {
			return encodeImage(data, mime, opt)
		}
	}
	return Result{}, errs.Unsup("thumbnail", string(fmt))
}

func decodeVP8(frame []byte, opt Options) (Result, error) {
	dec := vp8.NewDecoder()
	dec.Init(bytes.NewReader(frame), len(frame))
	ycbcr, err := dec.DecodeFrame()
	if err != nil {
		return Result{}, err
	}
	bounds := ycbcr.Bounds()
	img := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			yy := ycbcr.Y[y*ycbcr.YStride+x]
			cb := ycbcr.Cb[(y/2)*ycbcr.CStride+x/2]
			cr := ycbcr.Cr[(y/2)*ycbcr.CStride+x/2]
			r8, g8, b8 := color.YCbCrToRGB(yy, cb, cr)
			img.Set(x, y, color.RGBA{R: r8, G: g8, B: b8, A: 255})
		}
	}
	return rasterize(img, opt, "decode")
}

func encodeImage(data []byte, mime string, opt Options) (Result, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Result{}, err
	}
	return rasterize(img, opt, "embedded")
}

func rasterize(img image.Image, opt Options, src string) (Result, error) {
	img = scaleDown(img, opt.MaxWidth, opt.MaxHeight)
	var buf bytes.Buffer
	switch opt.Format {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return Result{}, err
		}
		return Result{MIME: "image/png", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	default:
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: opt.JPEGQuality}); err != nil {
			return Result{}, err
		}
		return Result{MIME: "image/jpeg", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	}
}

func scaleDown(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 && h > maxW {
		scale := float64(maxW) / float64(w)
		nh := int(float64(h) * scale)
		return resizeNearest(img, maxW, nh)
	}
	if w <= maxW && (maxH <= 0 || h <= maxH) {
		return img
	}
	scale := float64(maxW) / float64(w)
	nh := int(float64(h) * scale)
	if maxH > 0 && nh > maxH {
		scale = float64(maxH) / float64(h)
		maxW = int(float64(w) * scale)
		nh = maxH
	}
	return resizeNearest(img, maxW, nh)
}

func resizeNearest(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < h; y++ {
		sy := y * sh / h
		for x := 0; x < w; x++ {
			sx := x * sw / w
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
