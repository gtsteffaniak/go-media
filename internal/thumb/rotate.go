package thumb

import (
	"image"
	"image/draw"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
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

// orientation is the display transform still to apply to the scaled
// thumbnail: a counter-clockwise rotation followed by an optional
// left-to-right mirror. Any of the eight HEIF irot/imir combinations reduces
// to this form, because a top-to-bottom flip equals a 180° turn plus a
// left-to-right flip.
type orientation struct {
	rot   int  // degrees counter-clockwise (ffprobe sign convention)
	hflip bool // mirror left-to-right after rotating
}

// orientationFromHEIF maps HEIF irot/imir to an orientation the way ffmpeg's
// autorotate renders it (rotate first, then imir axis 0 = vflip, axis 1 =
// hflip). Verified against ffmpeg 9.0.2 for all 12 irot×imir combinations
// produced by avifenc --irot/--imir.
func orientationFromHEIF(t heifthumb.Transform) orientation {
	o := orientation{rot: normRotation(t.Rotation)}
	if t.Mirror {
		o.hflip = true
		if t.MirrorAxis == 0 {
			o.rot = normRotation(o.rot + 180)
		}
	}
	return o
}

// orientImage applies o to the (already scaled) thumbnail.
func orientImage(img image.Image, o orientation) image.Image {
	img = rotateImage(img, o.rot)
	if o.hflip {
		img = hflipImage(img)
	}
	return img
}

// hflipImage mirrors img left-to-right. YCbCr 4:4:4, and 4:2:0 with an even
// width, flip plane by plane; other images go through RGBA.
func hflipImage(img image.Image) image.Image {
	if ycc, ok := img.(*image.YCbCr); ok && ycc.Rect.Min == (image.Point{}) {
		w, h := ycc.Rect.Dx(), ycc.Rect.Dy()
		switch {
		case ycc.SubsampleRatio == image.YCbCrSubsampleRatio444:
			out := &image.YCbCr{SubsampleRatio: ycc.SubsampleRatio, Rect: ycc.Rect}
			out.Y, out.YStride = flipPlane(ycc.Y, ycc.YStride, w, h)
			out.Cb, out.CStride = flipPlane(ycc.Cb, ycc.CStride, w, h)
			out.Cr, _ = flipPlane(ycc.Cr, ycc.CStride, w, h)
			return out
		case ycc.SubsampleRatio == image.YCbCrSubsampleRatio420 && w%2 == 0:
			ch := (h + 1) / 2
			out := &image.YCbCr{SubsampleRatio: ycc.SubsampleRatio, Rect: ycc.Rect}
			out.Y, out.YStride = flipPlane(ycc.Y, ycc.YStride, w, h)
			out.Cb, out.CStride = flipPlane(ycc.Cb, ycc.CStride, w/2, ch)
			out.Cr, _ = flipPlane(ycc.Cr, ycc.CStride, w/2, ch)
			return out
		}
	}
	b := img.Bounds()
	src, ok := img.(*image.RGBA)
	if !ok || src.Rect.Min != (image.Point{}) {
		src = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(src, src.Rect, img, b.Min, draw.Src)
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewRGBA(src.Rect)
	for y := 0; y < h; y++ {
		s := src.Pix[y*src.Stride : y*src.Stride+4*w]
		d := dst.Pix[y*dst.Stride : y*dst.Stride+4*w]
		for x := 0; x < w; x++ {
			copy(d[4*(w-1-x):4*(w-x)], s[4*x:4*x+4])
		}
	}
	return dst
}

// flipPlane mirrors one w×h 8-bit plane left-to-right into a tight buffer.
func flipPlane(src []byte, stride, w, h int) ([]byte, int) {
	dst := make([]byte, w*h)
	for y := 0; y < h; y++ {
		s := src[y*stride : y*stride+w]
		d := dst[y*w : y*w+w]
		for x, v := range s {
			d[w-1-x] = v
		}
	}
	return dst, w
}
