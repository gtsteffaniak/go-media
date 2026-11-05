// Package jxldecode decodes JPEG XL still images with github.com/gen2brain/jxl.
//
// Decoding is pure Go; ffmpeg is not used as a fallback or reference for
// JPEG XL; the limits below are documented here.
//
// Memory bounds: Decode first reads the SizeHeader from the first 4 KiB and
// rejects images over the budget; otherwise jxl.DecodeConfig parses the header
// from a bounded prefix (at most [MaxProbeBytes]). Either way oversized images
// are rejected before the rest of the file is read. Only accepted files are read fully
// (up to 512 MiB) and decoded.
//
// Pixel cap: gen2brain/jxl has no downscaled decode, so the whole frame is
// always decoded, whatever box is requested. go-media sets no policy cap of
// its own; the caller chooses one with [Options.MaxPixels] (exposed publicly
// as media.ThumbnailOptions.MaxSourcePixels), and it applies identically to
// every box, including full resolution. With no caller cap, only
// gen2brain/jxl's own ceiling applies ([DecoderPixelLimit], 16384×16384, its
// default frame limit and the level-5 area limit). Images over the effective
// limit return an errs.Unsupported error naming the dimensions and the limit.
package jxldecode

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"io"

	"github.com/gen2brain/jxl"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"golang.org/x/image/draw"
)

const maxReadBytes = 512 * 1024 * 1024

// MaxProbeBytes bounds the prefix read to parse the JPEG XL header before the
// size check. jxl.DecodeConfig reads in growing chunks (4 KiB first), so a
// typical probe reads a few KiB; the cap only matters for huge ICC profiles.
const MaxProbeBytes = 1 << 20

// ErrTooLarge reports a JPEG XL file above maxReadBytes; it is not decoded.
var ErrTooLarge = errors.New("jxldecode: file exceeds 512 MiB size limit")

// DecoderPixelLimit is gen2brain/jxl's own frame-area ceiling
// (jxl.DefaultFrameSizeLimit, 16384×16384, also the level-5 area limit). It is
// a decoder limit, not a go-media policy, and applies when no caller cap is set.
const DecoderPixelLimit = jxl.DefaultFrameSizeLimit

// Options configures [DecodeWithOptions].
type Options struct {
	// MaxPixels caps the original image area (width×height) the caller is
	// willing to decode, independent of the requested box. Zero or negative
	// means no caller cap; values above DecoderPixelLimit are clamped to it.
	MaxPixels int64
}

// limit returns the effective pixel limit and a label naming its source.
func (o Options) limit() (int64, string) {
	if o.MaxPixels <= 0 || o.MaxPixels > DecoderPixelLimit {
		return DecoderPixelLimit, "decoder limit"
	}
	return o.MaxPixels, "caller MaxPixels cap"
}

// Decode is DecodeWithOptions with zero Options (no caller pixel cap).
func Decode(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, maxW, maxH int) (image.Image, error) {
	return DecodeWithOptions(r, size, maxW, maxH, Options{})
}

// DecodeWithOptions reads a JPEG XL file and returns a display-oriented
// raster fitted to maxW×maxH (0 means unbounded on that edge). The header is
// probed from a bounded prefix and the image rejected with errs.Unsupported if
// its area is over opt.MaxPixels (or DecoderPixelLimit); only then is the file
// read fully.
// JPEG XL codestreams carry display orientation; container EXIF orientation is ignored (ISO/IEC 18181).
func DecodeWithOptions(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, maxW, maxH int, opt Options) (image.Image, error) {
	if size <= 0 {
		return nil, jxl.ErrInvalid
	}
	if size > maxReadBytes {
		return nil, ErrTooLarge
	}
	budget, src := opt.limit()

	probeN := min(size, MaxProbeBytes)
	// Fast path: the SizeHeader sits in the first bytes, so check it on a
	// 4 KiB read before jxl.DecodeConfig. DecodeConfig keeps growing its
	// buffer on a header it refuses, which would read the whole 1 MiB prefix.
	head := make([]byte, min(probeN, 4096))
	if _, err := r.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if w, h, ok := headerSize(head); ok && int64(w)*int64(h) > budget {
		return nil, tooLarge(w, h, budget, src)
	}
	rest := io.NewSectionReader(r, int64(len(head)), probeN-int64(len(head)))
	cfg, err := jxl.DecodeConfig(io.MultiReader(bytes.NewReader(head), rest))
	if err != nil {
		if errors.Is(err, jxl.ErrUnsupported) {
			// Area is within budget here, so gen2brain/jxl refused another
			// level-5 limit (ISO/IEC 18181-2): an edge over 2^18 px, more
			// than 4 extra channels, a CMYK black channel, or ICC over 4 MiB.
			return nil, errs.Unsup("thumbnail", "jxl: codestream needs level 10 (edge over 262144 px, over 4 extra channels, CMYK, or ICC over 4 MiB)")
		}
		if probeN == size {
			return nil, err
		}
		// Header did not fit the probe prefix (e.g. a very large ICC
		// profile); fall through and let the decoder's frame limit apply.
	} else if px := int64(cfg.Width) * int64(cfg.Height); px > budget {
		return nil, tooLarge(cfg.Width, cfg.Height, budget, src)
	}

	data, err := ioat.ReadAll(r, size, 0, size)
	if err != nil {
		return nil, err
	}

	img, err := jxl.Decode(bytes.NewReader(data), jxl.Options{FrameSizeLimit: int(budget)})
	if err != nil {
		if errors.Is(err, jxl.ErrFrameTooLarge) {
			// A frame (not the image header) is over budget.
			return nil, errs.Unsup("thumbnail", fmt.Sprintf("jxl: frame exceeds %s of %d px (no downscaled decode)", src, budget))
		}
		return nil, err
	}

	if maxW > 0 || maxH > 0 {
		img = resizeFit(img, maxW, maxH)
	}
	return img, nil
}

func tooLarge(w, h int, budget int64, src string) error {
	return errs.Unsup("thumbnail", fmt.Sprintf("jxl: image %dx%d (%d px) exceeds %s of %d px (no downscaled decode)", w, h, int64(w)*int64(h), src, budget))
}

func resizeFit(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW == 0 || srcH == 0 {
		return src
	}

	targetW, targetH := srcW, srcH
	if maxW > 0 && maxH > 0 {
		scaleW := float64(maxW) / float64(srcW)
		scaleH := float64(maxH) / float64(srcH)
		scale := scaleW
		if scaleH < scale {
			scale = scaleH
		}
		if scale >= 1 {
			return src
		}
		targetW = int(float64(srcW)*scale + 0.5)
		targetH = int(float64(srcH)*scale + 0.5)
	} else if maxW > 0 {
		if srcW <= maxW {
			return src
		}
		targetH = int(float64(srcH)*float64(maxW)/float64(srcW) + 0.5)
		targetW = maxW
	} else if maxH > 0 {
		if srcH <= maxH {
			return src
		}
		targetW = int(float64(srcW)*float64(maxH)/float64(srcH) + 0.5)
		targetH = maxH
	}

	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}
