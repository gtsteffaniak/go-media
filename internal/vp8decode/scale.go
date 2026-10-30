package vp8decode

import (
	"image"

	gvp8 "github.com/gen2brain/vpx/vp8"
	"golang.org/x/image/draw"
)

// PictureRGBAScaled converts and scales a VP8 picture to fit inside maxW×maxH (even dimensions).
// scaler is thumb.ScalerMode as int; 0 defaults to balanced two-stage.
func PictureRGBAScaled(p *gvp8.Picture, maxW, maxH int, scaler int) *image.RGBA {
	if p == nil {
		return nil
	}
	w, h := p.Width, p.Height
	if maxW <= 0 {
		maxW = w
	}
	if maxH <= 0 {
		maxH = h
	}
	tw, th := fitSize(w, h, maxW, maxH)
	if tw == w && th == h {
		return pictureRGBA(p)
	}
	// Cheap integer box halving in YUV, then high-quality final step on smaller planes.
	sp := clonePicture(p)
	for sp.Width > tw*2 || sp.Height > th*2 {
		sp = halve420(sp)
	}
	if sp.Width == tw && sp.Height == th {
		out := image.NewRGBA(image.Rect(0, 0, tw, th))
		yuv420BT601LimitedToRGBA(out, sp)
		return out
	}
	mode := scaler
	if mode == 0 {
		mode = 2 // balanced
	}
	switch mode {
	case 1: // area — YUV area then RGB
		return scalePictureArea(sp, tw, th)
	case 3, 5: // high / catmullrom
		return scalePictureCatmull(sp, tw, th)
	case 4: // bilinear
		return scalePictureBilinear(sp, tw, th)
	case 2: // balanced: box halve in YUV then bilinear
		return scalePictureBilinear(sp, tw, th)
	default: // fast
		return scalePictureBilinear(sp, tw, th)
	}
}

func fitSize(w, h, maxW, maxH int) (int, int) {
	if w <= maxW && h <= maxH {
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
	if th%2 != 0 {
		th++
	}
	return tw, th
}

func clonePicture(p *gvp8.Picture) *gvp8.Picture {
	out := &gvp8.Picture{
		YStride:  p.YStride,
		UVStride: p.UVStride,
		Width:    p.Width,
		Height:   p.Height,
		Y:        append([]byte(nil), p.Y...),
		U:        append([]byte(nil), p.U...),
		V:        append([]byte(nil), p.V...),
	}
	return out
}

// halve420 box-filters YUV 4:2:0 to half resolution.
func halve420(p *gvp8.Picture) *gvp8.Picture {
	w, h := p.Width, p.Height
	nw, nh := w/2, h/2
	if nw < 2 || nh < 2 {
		return p
	}
	ncw := (nw + 1) / 2
	nch := (nh + 1) / 2
	out := &gvp8.Picture{
		Width: nw, Height: nh,
		YStride: nw, UVStride: ncw,
		Y: make([]byte, nw*nh),
		U: make([]byte, ncw*nch),
		V: make([]byte, ncw*nch),
	}
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			y0 := 2*y
			x0 := 2*x
			out.Y[y*out.YStride+x] = byte((int(p.Y[y0*p.YStride+x0]) + int(p.Y[y0*p.YStride+x0+1]) +
				int(p.Y[(y0+1)*p.YStride+x0]) + int(p.Y[(y0+1)*p.YStride+x0+1]) + 2) / 4)
		}
	}
	for y := 0; y < nch; y++ {
		for x := 0; x < ncw; x++ {
			y0 := 2 * y
			x0 := 2 * x
			out.U[y*out.UVStride+x] = avgChroma4(p.U, p.UVStride, x0, y0, p.Width, p.Height)
			out.V[y*out.UVStride+x] = avgChroma4(p.V, p.UVStride, x0, y0, p.Width, p.Height)
		}
	}
	return out
}

func avgChroma4(plane []byte, stride int, x0, y0, w, h int) byte {
	x1 := x0 + 1
	y1 := y0 + 1
	if x1 >= (w+1)/2 {
		x1 = x0
	}
	if y1 >= (h+1)/2 {
		y1 = y0
	}
	v00 := int(plane[y0*stride+x0])
	v10 := int(plane[y0*stride+x1])
	v01 := int(plane[y1*stride+x0])
	v11 := int(plane[y1*stride+x1])
	return byte((v00 + v10 + v01 + v11 + 2) / 4)
}

func scalePictureBilinear(p *gvp8.Picture, tw, th int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, tw, th))
	sw := float64(p.Width) / float64(tw)
	sh := float64(p.Height) / float64(th)
	for y := 0; y < th; y++ {
		sy := (float64(y)+0.5)*sh - 0.5
		y0 := int(sy)
		if y0 < 0 {
			y0 = 0
		}
		y1 := y0 + 1
		if y1 >= p.Height {
			y1 = p.Height - 1
		}
		fy := sy - float64(y0)
		for x := 0; x < tw; x++ {
			sx := (float64(x)+0.5)*sw - 0.5
			x0 := int(sx)
			if x0 < 0 {
				x0 = 0
			}
			x1 := x0 + 1
			if x1 >= p.Width {
				x1 = p.Width - 1
			}
			fx := sx - float64(x0)
			yy := bilerpY(p, x0, y0, x1, y1, fx, fy)
			cb := bilerpUV(p.U, p.UVStride, p.Width, p.Height, x0, y0, x1, y1, fx, fy)
			cr := bilerpUV(p.V, p.UVStride, p.Width, p.Height, x0, y0, x1, y1, fx, fy)
			c := float64(yy) - 16
			d := float64(cb) - 128
			e := float64(cr) - 128
			di := y*out.Stride + x*4
			out.Pix[di+0] = clamp8(1.164*c + 1.793*e)
			out.Pix[di+1] = clamp8(1.164*c - 0.213*d - 0.533*e)
			out.Pix[di+2] = clamp8(1.164*c + 2.112*d)
			out.Pix[di+3] = 255
		}
	}
	return out
}

func bilerpY(p *gvp8.Picture, x0, y0, x1, y1 int, fx, fy float64) byte {
	v00 := float64(p.Y[y0*p.YStride+x0])
	v10 := float64(p.Y[y0*p.YStride+x1])
	v01 := float64(p.Y[y1*p.YStride+x0])
	v11 := float64(p.Y[y1*p.YStride+x1])
	top := v00 + fx*(v10-v00)
	bot := v01 + fx*(v11-v01)
	return byte(top + fy*(bot-top) + 0.5)
}

func bilerpUV(plane []byte, stride int, w, h int, x0, y0, x1, y1 int, fx, fy float64) byte {
	ux0, uy0 := x0/2, y0/2
	ux1, uy1 := x1/2, y1/2
	v00 := float64(plane[uy0*stride+ux0])
	v10 := float64(plane[uy0*stride+ux1])
	v01 := float64(plane[uy1*stride+ux0])
	v11 := float64(plane[uy1*stride+ux1])
	top := v00 + fx*(v10-v00)
	bot := v01 + fx*(v11-v01)
	return byte(top + fy*(bot-top) + 0.5)
}

func scalePictureCatmull(p *gvp8.Picture, tw, th int) *image.RGBA {
	sp := clonePicture(p)
	for sp.Width > tw*2 || sp.Height > th*2 {
		sp = halve420(sp)
	}
	mid := image.NewRGBA(image.Rect(0, 0, sp.Width, sp.Height))
	yuv420BT601LimitedToRGBA(mid, sp)
	if sp.Width == tw && sp.Height == th {
		return mid
	}
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(dst, dst.Bounds(), mid, mid.Bounds(), draw.Over, nil)
	return dst
}
