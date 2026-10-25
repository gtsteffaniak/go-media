package thumb

import (
	"image"
	"image/color"
	"runtime"
	"sync"

	"github.com/gtsteffaniak/go-media/internal/h264"
)

type yccRGB func(yy, cb, cr byte) (r, g, b byte)

func yccSource(img image.Image) (*image.YCbCr, yccRGB) {
	if t, ok := img.(*h264.ThumbYCbCr); ok && t.YCbCr != nil {
		return t.YCbCr, t.RGB
	}
	if ycc, ok := img.(*image.YCbCr); ok {
		// JPEG/MJPEG decodes use full-range YCbCr; do not apply video limited-range matrices.
		return ycc, jpegYCbCrToRGBA
	}
	return nil, nil
}

// scaleYCbCrArea downscales YCbCr with tile averaging, then converts to RGBA.
func scaleYCbCrArea(src *image.YCbCr, dstW, dstH int, toRGB yccRGB) *image.RGBA {
	cur := src
	for cur.Bounds().Dx() > dstW*2 && cur.Bounds().Dy() > dstH*2 {
		cur = halveYCbCr(cur)
	}
	if dstW*dstH <= 512*512 {
		return scaleYCbCrAreaSerial(cur, dstW, dstH, toRGB)
	}
	sb := cur.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	workers := runtime.GOMAXPROCS(0)
	if workers > dstH {
		workers = dstH
	}
	var wg sync.WaitGroup
	rows := (dstH + workers - 1) / workers
	for w := 0; w < workers; w++ {
		y0 := w * rows
		y1 := y0 + rows
		if y1 > dstH {
			y1 = dstH
		}
		if y0 >= y1 {
			continue
		}
		wg.Add(1)
		go func(yStart, yEnd int) {
			defer wg.Done()
			for dy := yStart; dy < yEnd; dy++ {
				sy0 := sb.Min.Y + (dy*sh)/dstH
				sy1 := sb.Min.Y + ((dy+1)*sh)/dstH
				if sy1 <= sy0 {
					sy1 = sy0 + 1
				}
				for dx := 0; dx < dstW; dx++ {
					sx0 := sb.Min.X + (dx*sw)/dstW
					sx1 := sb.Min.X + ((dx+1)*sw)/dstW
					if sx1 <= sx0 {
						sx1 = sx0 + 1
					}
					var ySum, cbSum, crSum, n int
					for sy := sy0; sy < sy1; sy++ {
						for sx := sx0; sx < sx1; sx++ {
							yy := cur.Y[cur.YOffset(sx, sy)]
							cb, cr := sampleCbCr(cur, sx, sy)
							ySum += int(yy)
							cbSum += int(cb)
							crSum += int(cr)
							n++
						}
					}
					if n == 0 {
						n = 1
					}
					yy := byte((ySum + n/2) / n)
					cb := byte((cbSum + n/2) / n)
					cr := byte((crSum + n/2) / n)
					di := dy*dst.Stride + dx*4
					dst.Pix[di+0], dst.Pix[di+1], dst.Pix[di+2] = toRGB(yy, cb, cr)
					dst.Pix[di+3] = 255
				}
			}
		}(y0, y1)
	}
	wg.Wait()
	return dst
}

func scaleYCbCrAreaSerial(src *image.YCbCr, dstW, dstH int, toRGB yccRGB) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for dy := 0; dy < dstH; dy++ {
		sy0 := sb.Min.Y + (dy*sh)/dstH
		sy1 := sb.Min.Y + ((dy+1)*sh)/dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < dstW; dx++ {
			sx0 := sb.Min.X + (dx*sw)/dstW
			sx1 := sb.Min.X + ((dx+1)*sw)/dstW
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var ySum, cbSum, crSum, n int
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					yy := src.Y[src.YOffset(sx, sy)]
					cb, cr := sampleCbCr(src, sx, sy)
					ySum += int(yy)
					cbSum += int(cb)
					crSum += int(cr)
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			yy := byte((ySum + n/2) / n)
			cb := byte((cbSum + n/2) / n)
			cr := byte((crSum + n/2) / n)
			di := dy*dst.Stride + dx*4
			dst.Pix[di+0], dst.Pix[di+1], dst.Pix[di+2] = toRGB(yy, cb, cr)
			dst.Pix[di+3] = 255
		}
	}
	return dst
}

func halveYCbCr(src *image.YCbCr) *image.YCbCr {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	nw, nh := w/2, h/2
	if nw < 2 || nh < 2 {
		return src
	}
	dst := &image.YCbCr{
		Rect:           image.Rect(0, 0, nw, nh),
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		YStride:        nw,
		CStride:        nw / 2,
		Y:              make([]byte, nw*nh),
		Cb:             make([]byte, (nw/2)*(nh/2)),
		Cr:             make([]byte, (nw/2)*(nh/2)),
	}
	for y := 0; y < nh; y++ {
		for x := 0; x < nw; x++ {
			x0 := b.Min.X + 2*x
			y0 := b.Min.Y + 2*y
			dst.Y[y*dst.YStride+x] = byte((int(src.Y[src.YOffset(x0, y0)]) + int(src.Y[src.YOffset(x0+1, y0)]) +
				int(src.Y[src.YOffset(x0, y0+1)]) + int(src.Y[src.YOffset(x0+1, y0+1)]) + 2) / 4)
		}
	}
	for y := 0; y < nh/2; y++ {
		for x := 0; x < nw/2; x++ {
			lx := b.Min.X + 2*x
			ly := b.Min.Y + 2*y
			co := src.COffset(lx, ly)
			c1 := src.COffset(lx+2, ly)
			c2 := src.COffset(lx, ly+2)
			c3 := src.COffset(lx+2, ly+2)
			dst.Cb[y*dst.CStride+x] = byte((int(src.Cb[co]) + int(src.Cb[c1]) + int(src.Cb[c2]) + int(src.Cb[c3]) + 2) / 4)
			dst.Cr[y*dst.CStride+x] = byte((int(src.Cr[co]) + int(src.Cr[c1]) + int(src.Cr[c2]) + int(src.Cr[c3]) + 2) / 4)
		}
	}
	return dst
}

func sampleCbCr(src *image.YCbCr, x, y int) (byte, byte) {
	return src.Cb[src.COffset(x, y)], src.Cr[src.COffset(x, y)]
}

func clampU8(v float64) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v + 0.5)
}

// tryScaleYCbCrSource scales if img is YCbCr and target is smaller.
func tryScaleYCbCrSource(img image.Image, maxW, maxH int, mode ScalerMode) (image.Image, bool) {
	// Video IDR frames only; embedded JPEG/MJPEG YCbCr must scale in RGB (chroma siting differs).
	if _, ok := img.(*h264.ThumbYCbCr); !ok {
		return img, false
	}
	ycc, toRGB := yccSource(img)
	if ycc == nil {
		return img, false
	}
	tw, th := fitThumbSize(ycc.Bounds().Dx(), ycc.Bounds().Dy(), maxW, maxH)
	if tw >= ycc.Bounds().Dx() && th >= ycc.Bounds().Dy() {
		return img, false
	}
	if mode == ScalerArea || mode == ScalerBalanced || mode == ScalerHigh || mode == ScalerFast {
		return scaleYCbCrArea(ycc, tw, th, toRGB), true
	}
	return img, false
}

func jpegYCbCrToRGBA(yy, cb, cr byte) (r, g, b byte) {
	c := color.YCbCr{Y: yy, Cb: cb, Cr: cr}
	rr, gg, bb, _ := c.RGBA()
	return byte(rr >> 8), byte(gg >> 8), byte(bb >> 8)
}

// yuv420BT601LimitedToRGBA converts limited-range 4:2:0 samples to RGB (BT.601 default when VUI absent).
func yuv420BT601LimitedToRGBA(yy, cb, cr byte) (r, g, b byte) {
	y := float64(yy) - 16
	cbf := float64(cb) - 128
	crf := float64(cr) - 128
	return clampU8(1.164*y + 1.596*crf),
		clampU8(1.164*y - 0.391*cbf - 0.813*crf),
		clampU8(1.164*y + 2.018*cbf)
}

// rgbaFromImage uses fast YCbCr path when possible.
func rgbaFromImage(img image.Image) *image.RGBA {
	if ycc, toRGB := yccSource(img); ycc != nil {
		b := ycc.Bounds()
		dst := image.NewRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				yy := ycc.Y[ycc.YOffset(x, y)]
				cb := ycc.Cb[ycc.COffset(x, y)]
				cr := ycc.Cr[ycc.COffset(x, y)]
				r8, g8, b8 := toRGB(yy, cb, cr)
				dst.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: 255})
			}
		}
		return dst
	}
	return asRGBA(img)
}
