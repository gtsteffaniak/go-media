package vp8decode

import (
	"image"
	"image/color"

	gvp8 "github.com/gen2brain/vpx/vp8"
)

// yuv420BT601LimitedToRGBA writes an RGBA bitmap from 4:2:0 planes using BT.601 limited range.
//
// VP8 has no matrix signalling beyond color_space 0 = BT.601 (RFC 6386 §9.2),
// and ffmpeg's vp8 decoder leaves the frame colorspace unspecified, so swscale
// applies its default BT.601 limited-range table at every resolution. The old
// BT.709-for-everything choice did not match ffmpeg for any VP8 stream.
func yuv420BT601LimitedToRGBA(dst *image.RGBA, p *gvp8.Picture) {
	w, h := p.Width, p.Height
	dx0 := dst.Bounds().Min.X
	dy0 := dst.Bounds().Min.Y
	for y := 0; y < h; y++ {
		yOff := y * p.YStride
		uvRow := (y / 2) * p.UVStride
		dstRow := (dy0 + y) * dst.Stride
		for x := 0; x < w; x++ {
			yy := float64(p.Y[yOff+x]) - 16
			cb := float64(p.U[uvRow+x/2]) - 128
			cr := float64(p.V[uvRow+x/2]) - 128
			r := clamp8(1.164*yy + 1.596*cr)
			g := clamp8(1.164*yy - 0.391*cb - 0.813*cr)
			b := clamp8(1.164*yy + 2.018*cb)
			di := dstRow + (dx0+x)*4
			dst.Pix[di+0] = r
			dst.Pix[di+1] = g
			dst.Pix[di+2] = b
			dst.Pix[di+3] = 255
		}
	}
}

func clamp8(v float64) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v + 0.5)
}

// pictureRGBA converts a decoded VP8 picture to RGBA (BT.601 limited, as ffmpeg).
func pictureRGBA(p *gvp8.Picture) *image.RGBA {
	w, h := p.Width, p.Height
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	yuv420BT601LimitedToRGBA(out, p)
	return out
}

// Legacy full-range conversion kept for tests comparing matrix behavior.
func pictureRGBAFullRange(p *gvp8.Picture) *image.RGBA {
	w, h := p.Width, p.Height
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			yy := p.Y[y*p.YStride+x]
			cb := p.U[(y/2)*p.UVStride+(x/2)]
			cr := p.V[(y/2)*p.UVStride+x/2]
			r8, g8, b8 := color.YCbCrToRGB(yy, cb, cr)
			out.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: 255})
		}
	}
	return out
}
