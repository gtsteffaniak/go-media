package vp9decode

import (
	"image"

	"github.com/gtsteffaniak/govpx/pkg/govpx"
	"golang.org/x/image/draw"
)

func yuv420BT601LimitedToRGBA(dst *image.RGBA, p *govpx.Image) {
	w, h := p.Width, p.Height
	dx0 := dst.Bounds().Min.X
	dy0 := dst.Bounds().Min.Y
	for y := 0; y < h; y++ {
		yOff := y * p.YStride
		uvRow := (y / 2) * p.UStride
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

// PictureRGBA converts a decoded VP9 frame to RGBA (BT.601 limited, as ffmpeg).
func PictureRGBA(p *govpx.Image) *image.RGBA {
	if p == nil {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, p.Width, p.Height))
	yuv420BT601LimitedToRGBA(out, p)
	return out
}

// PictureRGBAScaled downscales then converts to RGBA.
func PictureRGBAScaled(p *govpx.Image, maxW, maxH, _ int) *image.RGBA {
	rgba := PictureRGBA(p)
	if rgba == nil {
		return nil
	}
	w, h := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 {
		maxH = h
	}
	tw, th := fitSize(w, h, maxW, maxH)
	if tw == w && th == h {
		return rgba
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(dst, dst.Bounds(), rgba, rgba.Bounds(), draw.Over, nil)
	return dst
}

func fitSize(w, h, maxW, maxH int) (int, int) {
	if w <= maxW && h <= maxH {
		return w, h
	}
	fw := float64(maxW) / float64(w)
	fh := float64(maxH) / float64(h)
	f := fw
	if fh < f {
		f = fh
	}
	tw := int(float64(w)*f + 0.5)
	th := int(float64(h)*f + 0.5)
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	if tw&1 != 0 {
		tw--
	}
	if th&1 != 0 {
		th--
	}
	if tw < 2 {
		tw = 2
	}
	if th < 2 {
		th = 2
	}
	return tw, th
}
