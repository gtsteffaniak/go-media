package vp8decode

import (
	"image"

	gvp8 "github.com/gen2brain/vpx/vp8"
)

// scalePictureArea scales YUV420 with tile averaging then converts to RGBA.
func scalePictureArea(p *gvp8.Picture, tw, th int) *image.RGBA {
	sw, sh := p.Width, p.Height
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for dy := 0; dy < th; dy++ {
		sy0 := (dy * sh) / th
		sy1 := ((dy + 1) * sh) / th
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for dx := 0; dx < tw; dx++ {
			sx0 := (dx * sw) / tw
			sx1 := ((dx + 1) * sw) / tw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var ySum, cbSum, crSum, n int
			for sy := sy0; sy < sy1; sy++ {
				yRow := sy * p.YStride
				uvRow := (sy / 2) * p.UVStride
				for sx := sx0; sx < sx1; sx++ {
					ySum += int(p.Y[yRow+sx])
					cbSum += int(p.U[uvRow+sx/2])
					crSum += int(p.V[uvRow+sx/2])
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			yy := float64(ySum/n) - 16
			cb := float64(cbSum/n) - 128
			cr := float64(crSum/n) - 128
			di := dy*dst.Stride + dx*4
			dst.Pix[di+0] = clamp8(1.164*yy + 1.793*cr)
			dst.Pix[di+1] = clamp8(1.164*yy - 0.213*cb - 0.533*cr)
			dst.Pix[di+2] = clamp8(1.164*yy + 2.112*cb)
			dst.Pix[di+3] = 255
		}
	}
	return dst
}
