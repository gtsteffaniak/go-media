// Package jxldecode decodes JPEG XL still images with github.com/gen2brain/jxl.
package jxldecode

import (
	"bytes"
	"errors"
	"fmt"
	"image"

	"github.com/gen2brain/jxl"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"golang.org/x/image/draw"
)

const maxReadBytes = 512 * 1024 * 1024

// ErrTooLarge reports a JPEG XL file above maxReadBytes; it is not decoded.
var ErrTooLarge = errors.New("jxldecode: file exceeds 512 MiB size limit")

// Bounds full-frame decode area for thumbnail vs full-raster callers.
const (
	maxThumbFramePixels  = 4096 * 4096
	maxRasterFramePixels = 8192 * 8192
)

// Decode reads a JPEG XL file and returns a display-oriented raster.
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
	data, err := ioat.ReadAll(r, size, 0, size)
	if err != nil {
		return nil, err
	}

	opts := jxl.Options{FrameSizeLimit: framePixelLimit(maxW, maxH)}
	img, err := jxl.Decode(bytes.NewReader(data), opts)
	if err != nil {
		if errors.Is(err, jxl.ErrFrameTooLarge) && (maxW > 0 || maxH > 0) {
			// gen2brain/jxl has no downscaled decode, so a frame over the thumbnail pixel
			// budget cannot be shrunk while decoding. Report that explicitly.
			return nil, fmt.Errorf("jxldecode: frame exceeds thumbnail pixel budget (%d px): %w", maxThumbFramePixels, err)
		}
		return nil, err
	}

	if maxW > 0 || maxH > 0 {
		img = resizeFit(img, maxW, maxH)
	}
	return img, nil
}

func framePixelLimit(maxW, maxH int) int {
	if maxW > 0 || maxH > 0 {
		return maxThumbFramePixels
	}
	return maxRasterFramePixels
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
