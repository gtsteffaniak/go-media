// Package jxldecode decodes JPEG XL still images with github.com/gen2brain/jxl.
package jxldecode

import (
	"bytes"
	"image"

	"github.com/gen2brain/jxl"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"golang.org/x/image/draw"
)

const maxReadBytes = 512 * 1024 * 1024

// Decode reads a JPEG XL file and returns a display-oriented raster.
func Decode(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, maxW, maxH int) (image.Image, error) {
	if size <= 0 {
		return nil, jxl.ErrInvalid
	}
	readSize := size
	if readSize > maxReadBytes {
		readSize = maxReadBytes
	}
	data, err := ioat.ReadAll(r, size, 0, readSize)
	if err != nil {
		return nil, err
	}

	img, err := jxl.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	if ex, exErr := jxl.DecodeExif(bytes.NewReader(data)); exErr == nil && ex != nil {
		img = applyExifOrientation(img, ex.Orientation)
	}

	if maxW > 0 || maxH > 0 {
		img = resizeFit(img, maxW, maxH)
	}
	return img, nil
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

func applyExifOrientation(src image.Image, orientation int) image.Image {
	switch orientation {
	case 1, 0:
		return src
	case 2:
		return flipHorizontal(src)
	case 3:
		return rotate180(src)
	case 4:
		return flipVertical(src)
	case 5:
		return rotate270(flipHorizontal(src))
	case 6:
		return rotate90(src)
	case 7:
		return rotate90(flipHorizontal(src))
	case 8:
		return rotate270(src)
	default:
		return src
	}
}

func rotate90(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate180(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func rotate270(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func flipHorizontal(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(w-1-x, y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func flipVertical(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst.Set(x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
