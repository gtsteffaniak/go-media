package thumb

import (
	"image"
	"image/draw"
)

// normRotation maps a display rotation to 0, 90, 180 or 270 degrees
// counter-clockwise (ffprobe sign convention: -90 is a clockwise quarter turn).
func normRotation(rot int) int {
	r := ((rot % 360) + 360) % 360
	switch r {
	case 90, 180, 270:
		return r
	}
	return 0
}

// swapsAxes reports whether rot turns the frame on its side.
func swapsAxes(rot int) bool {
	r := normRotation(rot)
	return r == 90 || r == 270
}

// rotateImage turns the (already scaled) thumbnail by rot so it is displayed
// upright, as ffmpeg's autorotate does. YCbCr 4:4:4, and 4:2:0 with even
// dimensions, rotate plane by plane; other images go through RGBA.
func rotateImage(img image.Image, rot int) image.Image {
	r := normRotation(rot)
	if r == 0 {
		return img
	}
	if ycc, ok := img.(*image.YCbCr); ok {
		even := ycc.Rect.Dx()%2 == 0 && ycc.Rect.Dy()%2 == 0 && ycc.Rect.Min.X%2 == 0 && ycc.Rect.Min.Y%2 == 0
		switch {
		case ycc.SubsampleRatio == image.YCbCrSubsampleRatio444,
			ycc.SubsampleRatio == image.YCbCrSubsampleRatio420 && even:
			return rotateYCbCr(ycc, r)
		}
	}
	src, ok := img.(*image.RGBA)
	if !ok || src.Rect.Min != (image.Point{}) {
		b := img.Bounds()
		src = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(src, src.Rect, img, b.Min, draw.Src)
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if r != 180 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		row := src.Pix[y*src.Stride : y*src.Stride+4*w]
		for x := 0; x < w; x++ {
			dx, dy := rotXY(x, y, w, h, r)
			copy(dst.Pix[dy*dst.Stride+4*dx:dy*dst.Stride+4*dx+4], row[4*x:4*x+4])
		}
	}
	return dst
}

// rotXY maps source pixel (x, y) of a w×h image to its position after a
// counter-clockwise rotation by r degrees.
func rotXY(x, y, w, h, r int) (int, int) {
	switch r {
	case 90: // counter-clockwise
		return y, w - 1 - x
	case 270: // clockwise
		return h - 1 - y, x
	default: // 180
		return w - 1 - x, h - 1 - y
	}
}

// rotatePlane rotates one w×h 8-bit plane by r degrees counter-clockwise.
func rotatePlane(src []byte, stride, w, h, r int) ([]byte, int) {
	dw := w
	if r != 180 {
		dw = h
	}
	dst := make([]byte, w*h)
	for y := 0; y < h; y++ {
		row := src[y*stride : y*stride+w]
		for x, v := range row {
			dx, dy := rotXY(x, y, w, h, r)
			dst[dy*dw+dx] = v
		}
	}
	return dst, dw
}

// rotateYCbCr rotates a 4:4:4 or even-sized 4:2:0 image; both stay valid
// after a quarter turn.
func rotateYCbCr(src *image.YCbCr, r int) *image.YCbCr {
	b := src.Rect
	w, h := b.Dx(), b.Dy()
	cw, ch := w, h
	if src.SubsampleRatio == image.YCbCrSubsampleRatio420 {
		cw, ch = w/2, h/2
	}
	yOff := src.YOffset(b.Min.X, b.Min.Y)
	cOff := src.COffset(b.Min.X, b.Min.Y)
	y, ys := rotatePlane(src.Y[yOff:], src.YStride, w, h, r)
	cb, cs := rotatePlane(src.Cb[cOff:], src.CStride, cw, ch, r)
	cr, _ := rotatePlane(src.Cr[cOff:], src.CStride, cw, ch, r)
	dw, dh := w, h
	if r != 180 {
		dw, dh = h, w
	}
	return &image.YCbCr{
		Y: y, Cb: cb, Cr: cr, YStride: ys, CStride: cs,
		SubsampleRatio: src.SubsampleRatio,
		Rect:           image.Rect(0, 0, dw, dh),
	}
}
