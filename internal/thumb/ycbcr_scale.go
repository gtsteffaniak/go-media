package thumb

import (
	"image"
	"image/color"
	"runtime"
	"sync"
)

// scaleYCbCrArea downscales JPEG/JFIF YCbCr with tile averaging and BT.601 full-range RGB (color.YCbCrToRGB).
func scaleYCbCrArea(src *image.YCbCr, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	sx0 := make([]int, dstW)
	sx1 := make([]int, dstW)
	for dx := 0; dx < dstW; dx++ {
		sx0[dx] = sb.Min.X + (dx*sw)/dstW
		sx1[dx] = sb.Min.X + ((dx+1)*sw)/dstW
		if sx1[dx] <= sx0[dx] {
			sx1[dx] = sx0[dx] + 1
		}
	}
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
				cy0 := sy0 / 2
				cy1 := (sy1 + 1) / 2
				dstRow := dy * dst.Stride
				for dx := 0; dx < dstW; dx++ {
					x0, x1 := sx0[dx], sx1[dx]
					var ySum, n int
					for sy := sy0; sy < sy1; sy++ {
						yRow := sy * src.YStride
						for sx := x0; sx < x1; sx++ {
							ySum += int(src.Y[yRow+sx])
							n++
						}
					}
					if n == 0 {
						n = 1
					}
					cx0 := x0 / 2
					cx1 := (x1 + 1) / 2
					var cbSum, crSum, cn int
					for cy := cy0; cy < cy1; cy++ {
						cRow := cy * src.CStride
						for cx := cx0; cx < cx1; cx++ {
							cbSum += int(src.Cb[cRow+cx])
							crSum += int(src.Cr[cRow+cx])
							cn++
						}
					}
					if cn == 0 {
						cn = 1
					}
					yy := byte(ySum / n)
					cb := byte(cbSum / cn)
					cr := byte(crSum / cn)
					r8, g8, b8 := color.YCbCrToRGB(yy, cb, cr)
					di := dstRow + dx*4
					dst.Pix[di+0] = r8
					dst.Pix[di+1] = g8
					dst.Pix[di+2] = b8
					dst.Pix[di+3] = 255
				}
			}
		}(y0, y1)
	}
	wg.Wait()
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
	ycc, ok := img.(*image.YCbCr)
	if !ok {
		return img, false
	}
	tw, th := fitThumbSize(ycc.Bounds().Dx(), ycc.Bounds().Dy(), maxW, maxH)
	if tw >= ycc.Bounds().Dx() && th >= ycc.Bounds().Dy() {
		return img, false
	}
	if mode == ScalerArea || mode == ScalerBalanced || mode == ScalerHigh || mode == ScalerFast {
		return scaleYCbCrArea(ycc, tw, th), true
	}
	return img, false
}

// rgbaFromImage uses fast YCbCr path when possible.
func rgbaFromImage(img image.Image) *image.RGBA {
	if ycc, ok := img.(*image.YCbCr); ok {
		b := ycc.Bounds()
		dst := image.NewRGBA(b)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				yy := ycc.Y[ycc.YOffset(x, y)]
				cb := ycc.Cb[ycc.COffset(x, y)]
				cr := ycc.Cr[ycc.COffset(x, y)]
				r8, g8, b8 := color.YCbCrToRGB(yy, cb, cr)
				dst.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: 255})
			}
		}
		return dst
	}
	return asRGBA(img)
}
