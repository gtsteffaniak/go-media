package thumb

import (
	"image"

	"golang.org/x/image/draw"
)

// scaleDown fits img inside maxW×maxH preserving aspect ratio.
func scaleDown(img image.Image, maxW, maxH int, filter Filter) image.Image {
	opt := Options{Filter: filter}
	return scaleImage(img, maxW, maxH, scalerFromOptions(opt))
}

// Filter is the downscale algorithm.
type Filter int

const (
	FilterFast Filter = 0
	FilterHigh Filter = 1
)

func filterFromOptions(opt Options) Filter {
	if opt.Filter == FilterHigh {
		return FilterHigh
	}
	if opt.Fast {
		return FilterFast
	}
	return FilterHigh
}

func asRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}
