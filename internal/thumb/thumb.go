// Package thumb supports go-media parsers.

package thumb

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"slices"
	"strings"

	"github.com/gen2brain/vpx/webp"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/format"
	"github.com/gtsteffaniak/go-media/internal/h264"
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
	img, src, rot, err := sourceImage(r, size, opt)
	if err != nil {
		return Result{}, err
	}
	return encodeImageRaster(img, fitOptions(opt, rot), src, rot)
}

// fitOptions returns opt with MaxWidth and MaxHeight swapped when rot turns
// the frame on its side, so the coded frame is fitted to the display box.
func fitOptions(opt Options, rot int) Options {
	if swapsAxes(rot) {
		opt.MaxWidth, opt.MaxHeight = opt.MaxHeight, opt.MaxWidth
	}
	return opt
}

// sourceImage decodes embedded art or a video frame without encoding. rot is
// the display rotation still to apply to the scaled image (MP4 tkhd matrix).
func sourceImage(r interface{ ReadAt([]byte, int64) (int, error) }, size int64, opt Options) (image.Image, string, int, error) {
	var rot int
	img, src, err := decodeSource(r, size, opt, &rot)
	if err != nil {
		return nil, "", 0, err
	}
	return img, src, rot, nil
}

// decodeSource does the work of sourceImage, storing the MP4 rotation in rot.
func decodeSource(r interface{ ReadAt([]byte, int64) (int, error) }, size int64, opt Options, rot *int) (image.Image, string, error) {
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
		// One bounded moov header walk serves the rotation check and the
		// MJPEG/H.264 seek. Rotation is applied by Generate after scaling.
		src, ok := isobmff.OpenThumbSource(r, size)
		if !ok {
			break
		}
		*rot = src.Rotation()
		opt = fitOptions(opt, *rot)
		if b, ok := src.MJPEGFrame(opt.SeekPercent); ok {
			img, err := decodeBytes(b, "image/jpeg", opt.MaxWidth, opt.MaxHeight, opt.Fast)
			if err != nil {
				return nil, "", err
			}
			return img, "decode", nil
		}
		// Intra-only IDR decode (fast preview / skip deblock). YCbCr → matrix-aware area downscale → JPEG.
		if useH264RGBPipeline(opt) {
			if rgba, err := src.H264ThumbnailRGBA(opt.SeekPercent, true); err != nil {
				return nil, "", err
			} else if rgba != nil {
				return rgba, "decode", nil
			}
		} else if yimg, err := src.H264ThumbnailYCbCr(opt.SeekPercent, true); err != nil {
			return nil, "", err
		} else if yimg != nil {
			return yimg, "decode", nil
		}
	case format.MKV, format.WebM:
		// Probe the bounded header once and decide from it before any demux pass.
		// Demuxing a multi-GB file only to find no usable track costs several
		// full-file reads. The header is passed down so the decoders do not re-read it.
		hdr, err := matroska.ProbeHeader(r, size)
		if err != nil {
			return nil, "", errs.Unsup("thumbnail", "matroska header unreadable")
		}
		if len(hdr.VideoCodecIDs) == 0 {
			return nil, "", errs.Unsup("thumbnail", "matroska: no video track in bounded header (unknown-size cluster, truncated, or no Segment)")
		}
		if !hasThumbCodec(hdr.VideoCodecIDs) {
			if slices.Contains(hdr.VideoCodecIDs, "V_MPEG4/ISO/AVC") {
				return nil, "", errs.UnsupProfile("thumbnail", "h264 in matroska")
			}
			return nil, "", errs.Unsup("thumbnail", "matroska video codec "+strings.Join(hdr.VideoCodecIDs, ","))
		}
		// VP8 and MJPEG seeking both map a percentage to time, so they need a Duration.
		if !hdr.HasDuration() {
			return nil, "", errs.Unsup("thumbnail", "matroska: no duration, seek impossible")
		}
		jpegOut := opt.Format == "" || opt.Format == "jpeg"
		if jpegOut {
			if pic, ok := matroska.VP8PictureAtSeek(r, size, hdr, opt.SeekPercent); ok {
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
		if img, ok := matroska.ThumbnailRGBA(r, size, hdr, tp); ok {
			return img, "decode", nil
		}
		// H.264 is decided from the IDs already parsed; no extra header read.
		if slices.Contains(hdr.VideoCodecIDs, "V_MPEG4/ISO/AVC") {
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

// hasThumbCodec reports whether any Matroska video codec is one the thumbnail
// path decodes (VP8 or MJPEG).
func hasThumbCodec(ids []string) bool {
	for _, id := range ids {
		if id == "V_VP8" || id == "V_MJPEG" {
			return true
		}
	}
	return false
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

// encodeImageRaster scales img to fit opt, turns it by rot, and encodes it.
func encodeImageRaster(img image.Image, opt Options, src string, rot int) (Result, error) {
	mode := scalerFromOptions(opt)
	jpegOut := opt.Format == "" || opt.Format == "jpeg"
	if jpegOut {
		if _, ok := img.(*h264.ThumbYCbCr); ok {
			if scaled, ok := tryScaleYCbCrSource(img, opt.MaxWidth, opt.MaxHeight, mode); ok {
				img = scaled
			} else {
				img = rgbaFromImage(img)
			}
		} else if scaled, ok := tryScaleYCbCrDown(img, opt.MaxWidth, opt.MaxHeight, mode); ok {
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
	img = rotateImage(img, rot)
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
