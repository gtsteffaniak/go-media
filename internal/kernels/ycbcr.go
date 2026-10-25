// Copyright 2026 Graham Steffaniak. MIT License.

package kernels

import "image/color"

// YCbCrToRGBFullRange converts JFIF full-range YCbCr to RGB (matches image/color.YCbCrToRGB).
func YCbCrToRGBFullRange(yy, cb, cr byte) (r, g, b byte) {
	return color.YCbCrToRGB(yy, cb, cr)
}
