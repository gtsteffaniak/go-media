package thumb

import (
	"image"
)

// ScalerMode selects the downscale algorithm.
type ScalerMode int

const (
	// ScalerFast is x/image/draw ApproxBiLinear on RGBA (legacy small preset).
	ScalerFast ScalerMode = 0
	// ScalerArea averages source tiles per output pixel (ffmpeg area-like).
	ScalerArea ScalerMode = 1
	// ScalerBalanced integer box/area down to ~2× target, then bilinear.
	ScalerBalanced ScalerMode = 2
	// ScalerHigh integer box/area down to ~2× target, then Catmull-Rom.
	ScalerHigh ScalerMode = 3
	// ScalerBiLinear is x/image/draw BiLinear.
	ScalerBiLinear ScalerMode = 4
	// ScalerCatmullRom is x/image/draw CatmullRom without prescale.
	ScalerCatmullRom ScalerMode = 5
)

// ScalerName returns a stable report label.
func ScalerName(m ScalerMode) string {
	switch m {
	case ScalerFast:
		return "fast"
	case ScalerArea:
		return "area"
	case ScalerBalanced:
		return "balanced"
	case ScalerHigh:
		return "high"
	case ScalerBiLinear:
		return "bilinear"
	case ScalerCatmullRom:
		return "catmullrom"
	default:
		return "unknown"
	}
}

// AllScalerModes lists modes benchmarked in the comparison harness.
func AllScalerModes() []ScalerMode {
	return []ScalerMode{
		ScalerArea,
		ScalerFast,
		ScalerBiLinear,
		ScalerBalanced,
		ScalerHigh,
		ScalerCatmullRom,
	}
}

func scalerFromOptions(opt Options) ScalerMode {
	if opt.Scaler != 0 {
		return opt.Scaler
	}
	switch opt.Filter {
	case FilterFast:
		return ScalerFast
	case FilterHigh:
		return ScalerHigh
	default:
		if opt.Fast {
			return ScalerFast
		}
		return ScalerHigh
	}
}

// scaleImage fits src inside maxW×maxH using mode.
func scaleImage(src image.Image, maxW, maxH int, mode ScalerMode) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	tw, th := fitThumbSize(w, h, maxW, maxH)
	if tw >= w && th >= h {
		return src
	}
	rgba := asRGBA(src)
	switch mode {
	case ScalerArea:
		return scaleAreaRGBA(rgba, tw, th)
	case ScalerFast:
		return resizeDraw(rgba, tw, th, true)
	case ScalerBiLinear:
		return resizeDraw(rgba, tw, th, false)
	case ScalerCatmullRom:
		return resizeCatmull(rgba, tw, th)
	case ScalerBalanced:
		return scaleTwoStage(rgba, tw, th, true)
	default: // ScalerHigh
		return scaleTwoStage(rgba, tw, th, false)
	}
}

func fitThumbSize(w, h, maxW, maxH int) (int, int) {
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 {
		maxH = h
	}
	if w <= maxW && h <= maxH {
		if w%2 != 0 {
			w++
		}
		if h%2 != 0 {
			h++
		}
		return w, h
	}
	sw := float64(maxW) / float64(w)
	sh := float64(maxH) / float64(h)
	s := sw
	if sh < s {
		s = sh
	}
	tw := int(float64(w) * s)
	th := int(float64(h) * s)
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	if tw%2 != 0 {
		tw++
	}
	if th%2 != 0 {
		th++
	}
	return tw, th
}

func scaleTwoStage(src *image.RGBA, tw, th int, bilinear bool) *image.RGBA {
	cur := src
	for cur.Bounds().Dx() > tw*2 || cur.Bounds().Dy() > th*2 {
		cur = boxHalfRGBA(cur)
	}
	if bilinear {
		return resizeDraw(cur, tw, th, false)
	}
	return resizeCatmull(cur, tw, th)
}
