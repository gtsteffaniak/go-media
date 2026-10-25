package thumb

import (
	"image"

	"golang.org/x/image/draw"
)

// scaleDown fits img inside maxW×maxH preserving aspect ratio.
func scaleDown(img image.Image, maxW, maxH int, filter Filter) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 {
		maxH = h
	}
	if w <= maxW && h <= maxH {
		return img
	}
	scaleW := float64(maxW) / float64(w)
	scaleH := float64(maxH) / float64(h)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}
	if scale >= 1 {
		return img
	}
	nw := int(float64(w) * scale)
	nh := int(float64(h) * scale)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	// Match ffmpeg scale=-2: even height for 4:2:0-friendly downstream encodes.
	if nh%2 != 0 {
		nh++
	}
	return resize(img, nw, nh, filter)
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

func resize(src image.Image, w, h int, filter Filter) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	var scaler draw.Interpolator
	switch filter {
	case FilterFast:
		scaler = draw.ApproxBiLinear
	default:
		scaler = draw.CatmullRom
	}
	scaler.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}
