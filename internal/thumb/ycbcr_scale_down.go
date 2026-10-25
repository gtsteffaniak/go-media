package thumb

import (
	"image"
	"runtime"
	"sync"

	"github.com/gtsteffaniak/go-media/internal/kernels"
)

// scaleYCbCrAreaDown returns a downscaled YCbCr image (area average), keeping JPEG fast paths in YCbCr.
func scaleYCbCrAreaDown(src *image.YCbCr, dstW, dstH int) *image.YCbCr {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewYCbCr(image.Rect(0, 0, dstW, dstH), src.SubsampleRatio)
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
	if dstW*dstH <= 256*256 {
		workers = 1
	} else if workers > dstH {
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
					x0, x1 := sx0[dx], sx1[dx]
					x := dx
					y := dy
					kernels.AreaDownRowY(dst.Y, dst.YStride, dst.YOffset(x, y), src.Y, src.YStride, sy0, sy1, x0, x1)
					kernels.AreaDownChromaTile(dst.Cb, dst.Cr, dst.CStride, x, y, src.Cb, src.Cr, src.CStride, sy0, sy1, x0, x1)
				}
			}
		}(y0, y1)
	}
	wg.Wait()
	return dst
}

// tryScaleYCbCrDown scales JPEG sources in YCbCr when the output is JPEG.
func tryScaleYCbCrDown(img image.Image, maxW, maxH int, mode ScalerMode) (image.Image, bool) {
	ycc, ok := img.(*image.YCbCr)
	if !ok {
		return img, false
	}
	tw, th := fitThumbSize(ycc.Bounds().Dx(), ycc.Bounds().Dy(), maxW, maxH)
	if tw >= ycc.Bounds().Dx() && th >= ycc.Bounds().Dy() {
		return img, false
	}
	if mode == ScalerArea || mode == ScalerBalanced || mode == ScalerHigh || mode == ScalerFast {
		return scaleYCbCrAreaDown(ycc, tw, th), true
	}
	return img, false
}

