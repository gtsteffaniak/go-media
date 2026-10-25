package thumb

import (
	"image"
	"runtime"
	"sync"
)

// scaleAreaRGBA downscales with per-output-pixel tile averaging (integer sums).
func scaleAreaRGBA(src *image.RGBA, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	if dstW <= 0 || dstH <= 0 {
		return dst
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
				srcY0 := sb.Min.Y + (dy*sh)/dstH
				srcY1 := sb.Min.Y + ((dy+1)*sh)/dstH
				if srcY1 <= srcY0 {
					srcY1 = srcY0 + 1
				}
				dstRow := dy * dst.Stride
				for dx := 0; dx < dstW; dx++ {
					srcX0 := sb.Min.X + (dx*sw)/dstW
					srcX1 := sb.Min.X + ((dx+1)*sw)/dstW
					if srcX1 <= srcX0 {
						srcX1 = srcX0 + 1
					}
					var pr, pg, pb, pa, n int
					for sy := srcY0; sy < srcY1; sy++ {
						row := sy * src.Stride
						for sx := srcX0; sx < srcX1; sx++ {
							si := row + sx*4
							pr += int(src.Pix[si])
							pg += int(src.Pix[si+1])
							pb += int(src.Pix[si+2])
							pa += int(src.Pix[si+3])
							n++
						}
					}
					if n == 0 {
						n = 1
					}
					di := dstRow + dx*4
					dst.Pix[di+0] = byte(pr / n)
					dst.Pix[di+1] = byte(pg / n)
					dst.Pix[di+2] = byte(pb / n)
					dst.Pix[di+3] = byte(pa / n)
				}
			}
		}(y0, y1)
	}
	wg.Wait()
	return dst
}
