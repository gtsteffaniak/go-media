// Package jxldecode decodes JPEG XL still images with github.com/gen2brain/jxl.
//
// Decoding is pure Go; ffmpeg is not used as a fallback or reference for
// JPEG XL, so the limits below are this package's own and are documented here.
//
// Memory bounds: Decode first reads the SizeHeader from the first 4 KiB and
// rejects images over the budget; otherwise jxl.DecodeConfig parses the header
// from a bounded prefix (at most [MaxProbeBytes]). Either way oversized images
// are rejected before the rest of the file is read. Only accepted files are read fully
// (up to 512 MiB) and decoded.
//
// Pixel budget: gen2brain/jxl has no downscaled decode, so the whole frame is
// always decoded. Every request, whatever its box (thumbnail or full raster),
// uses the same original-image budget, [MaxImagePixels] (4096×4096,
// 16,777,216 px), so a decode never allocates more than about 64 MiB (8-bit)
// or 128 MiB (16-bit) of RGBA. Images over it, at any box size including full
// resolution, return an errs.Unsupported error naming the dimensions and the
// budget.
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

// MaxImagePixels is the original-image pixel budget for every request
// (4096×4096 = 16,777,216 px). It does not depend on the requested box and
// applies to full-resolution requests too. It is far below gen2brain/jxl's
// default and level-5 limit (16384×16384), so the decoder's frame limit is
// set to it as well.
const MaxImagePixels = 4096 * 4096

// Decode reads a JPEG XL file and returns a display-oriented raster fitted to
// maxW×maxH (0 means unbounded on that edge). The header is probed from a
// bounded prefix and the image rejected with errs.Unsupported if its area is
// over [MaxImagePixels]; only then is the file read fully.
// JPEG XL codestreams carry display orientation; container EXIF orientation is ignored (ISO/IEC 18181).
func Decode(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, maxW, maxH int) (image.Image, error) {
	if size <= 0 {
		return nil, jxl.ErrInvalid
	}
	if size > maxReadBytes {
		return nil, ErrTooLarge
	}
	const budget = MaxImagePixels

	probeN := min(size, MaxProbeBytes)
	// Fast path: the SizeHeader sits in the first bytes, so check it on a
	// 4 KiB read before jxl.DecodeConfig. DecodeConfig keeps growing its
	// buffer on a header it refuses, which would read the whole 1 MiB prefix.
	head := make([]byte, min(probeN, 4096))
	if _, err := r.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if w, h, ok := headerSize(head); ok && int64(w)*int64(h) > budget {
		return nil, tooLarge(w, h, budget)
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
		return nil, tooLarge(cfg.Width, cfg.Height, budget)
	}

	data, err := ioat.ReadAll(r, size, 0, size)
	if err != nil {
		return nil, err
	}

	img, err := jxl.Decode(bytes.NewReader(data), jxl.Options{FrameSizeLimit: budget})
	if err != nil {
		if errors.Is(err, jxl.ErrFrameTooLarge) {
			// A frame (not the image header) is over budget.
			return nil, errs.Unsup("thumbnail", fmt.Sprintf("jxl: frame exceeds pixel budget of %d px (no downscaled decode)", budget))
		}
		return nil, err
	}

	if maxW > 0 || maxH > 0 {
		img = resizeFit(img, maxW, maxH)
	}
	return img, nil
}

func tooLarge(w, h int, budget int64) error {
	return errs.Unsup("thumbnail", fmt.Sprintf("jxl: image %dx%d (%d px) exceeds pixel budget of %d px (no downscaled decode)", w, h, int64(w)*int64(h), budget))
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
