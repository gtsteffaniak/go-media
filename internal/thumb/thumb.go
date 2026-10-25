// Package thumb supports go-media parsers.

package thumb

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/gen2brain/vpx/webp"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/matroska"
	"github.com/gtsteffaniak/go-media/internal/sniff"
	"github.com/gtsteffaniak/go-media/internal/vp8decode"
	"github.com/gtsteffaniak/go-media/internal/tagread"
)

// Options configures thumbnail generation.
type Options struct {
	MaxWidth       int
	MaxHeight      int
	SeekPercent    float64
	Format         string // jpeg, png, webp-lossless
	JPEGQuality    int    // 1–100; used when Quality is zero
	Quality        int    // 1–100 preferred JPEG quality
	Filter         Filter
	Scaler         ScalerMode
	Fast           bool
	PreferEmbedded bool
}

// Result is a data type used by this package.
type Result struct {
	MIME   string
	Data   []byte
	Source string
	Width  int
	Height int
}

// Generate supports media inspection.
func Generate(r interface{ ReadAt([]byte, int64) (int, error) }, size int64, opt Options) (Result, error) {
	normalizeOptions(&opt)
	img, src, err := sourceImage(r, size, opt)
	if err != nil {
		return Result{}, err
	}
	return encodeImageRaster(img, opt, src)
}

// sourceImage decodes embedded art or a video frame without encoding.
func sourceImage(r interface{ ReadAt([]byte, int64) (int, error) }, size int64, opt Options) (image.Image, string, error) {
	if opt.PreferEmbedded {
		if data, mime, err := tagread.Artwork(r, size); err == nil {
			img, err := decodeBytes(data, mime, opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err == nil {
				return img, "embedded", nil
			}
		}
	}

	fmtID, err := sniff.Detect(r, size)
	if err != nil {
		return nil, "", err
	}

	switch fmtID {
	case format.MP4:
		if b, ok := isobmff.MJPEGFrame(r, size, opt.SeekPercent); ok {
			img, err := decodeBytes(b, "image/jpeg", opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err != nil {
				return nil, "", err
			}
			return img, "decode", nil
		}
		// Intra-only IDR decode: skip deblocking for thumbnail latency (see README fast preview).
		if yimg, err := isobmff.H264ThumbnailYCbCr(r, size, opt.SeekPercent, true); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
	case format.MKV, format.WebM:
		jpegOut := opt.Format == "" || opt.Format == "jpeg"
		if jpegOut {
			if pic, ok := matroska.VP8PictureAtSeek(r, size, opt.SeekPercent); ok {
				if ycc := vp8decode.PictureYCbCr(pic); ycc != nil {
					return ycc, "decode", nil
				}
			}
		}
		tp := matroska.ThumbnailParams{
			SeekPercent: opt.SeekPercent,
			MaxWidth:    opt.MaxWidth,
			MaxHeight:   opt.MaxHeight,
			Scaler:      int(scalerFromOptions(opt)),
		}
		if img, ok := matroska.ThumbnailRGBA(r, size, tp); ok {
			return img, "decode", nil
		}
		if matroska.HasH264Video(r, size) {
			return nil, "", errs.UnsupProfile("thumbnail", "h264 in matroska")
		}
	case format.MP3, format.FLAC, format.OGG:
		if data, mime, err := tagread.Artwork(r, size); err == nil {
			img, err := decodeBytes(data, mime, opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err == nil {
				return img, "embedded", nil
			}
		}
	}
	return nil, "", errs.Unsup("thumbnail", string(fmtID))
}

func decodeBytes(data []byte, mime string, maxW, maxH int, fast bool) (image.Image, error) {
	img, _, err := jpegDecode(data, maxW, maxH, fast)
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return nil, errs.Unsup("thumbnail", "unsupported cover image format")
		}
		return nil, err
	}
	return img, nil
}

func encodeImageRaster(img image.Image, opt Options, src string) (Result, error) {
	mode := scalerFromOptions(opt)
	jpegOut := opt.Format == "" || opt.Format == "jpeg"
	if jpegOut {
		if scaled, ok := tryScaleYCbCrDown(img, opt.MaxWidth, opt.MaxHeight, mode); ok {
			img = scaled
		}
	}
	if _, yccOK := img.(*image.YCbCr); !yccOK || !jpegOut {
		if scaled, ok := tryScaleYCbCrSource(img, opt.MaxWidth, opt.MaxHeight, mode); ok {
			img = scaled
		} else if needsScale(img, opt.MaxWidth, opt.MaxHeight) {
			img = scaleImage(img, opt.MaxWidth, opt.MaxHeight, mode)
		}
	}
	var buf bytes.Buffer
	if jpegOut {
		b := img.Bounds()
		buf.Grow(b.Dx() * b.Dy() / 4)
	}
	switch opt.Format {
	case "png":
		if err := png.Encode(&buf, img); err != nil {
			return Result{}, err
		}
		return Result{MIME: "image/png", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	case "webp-lossless", "webp":
		if err := webp.Encode(&buf, img, webp.EncodeOptions{Lossless: true, Quality: 100}); err != nil {
			return Result{}, err
		}
		return Result{MIME: "image/webp", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
	}
	q := jpegQuality(opt)
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		return Result{}, err
	}
	return Result{MIME: "image/jpeg", Data: buf.Bytes(), Source: src, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}, nil
}

func jpegQuality(opt Options) int {
	if opt.Quality > 0 {
		return clampQuality(opt.Quality)
	}
	return clampQuality(opt.JPEGQuality)
}

func clampQuality(q int) int {
	if q < 1 {
		return 1
	}
	if q > 100 {
		return 100
	}
	return q
}

func needsScale(img image.Image, maxW, maxH int) bool {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 {
		maxH = h
	}
	return w > maxW || h > maxH
}

func normalizeOptions(opt *Options) {
	if opt.MaxWidth <= 0 {
		opt.MaxWidth = 320
	}
	if opt.SeekPercent <= 0 {
		opt.SeekPercent = 10
	}
	if opt.Quality > 0 {
		opt.JPEGQuality = opt.Quality
	}
	if opt.JPEGQuality <= 0 {
		opt.JPEGQuality = 85
	}
	if opt.Format == "" {
		opt.Format = "jpeg"
	}
}
