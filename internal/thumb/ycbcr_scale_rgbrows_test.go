package thumb

import (
	"bytes"
	"image"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

func TestScaleYCbCrAreaRGBRowsMatchesTileLoop(t *testing.T) {
	seed := uint32(5)
	rnd := func() byte { seed = seed*1664525 + 1013904223; return byte(seed >> 24) }
	for _, sz := range [][4]int{{1920, 1080, 320, 180}, {1918, 1078, 480, 270}, {101, 67, 17, 9}, {640, 360, 639, 359}, {1920, 1080, 1024, 576}} {
		img := image.NewYCbCr(image.Rect(0, 0, sz[0], sz[1]), image.YCbCrSubsampleRatio420)
		for i := range img.Y {
			img.Y[i] = rnd()
		}
		for i := range img.Cb {
			img.Cb[i], img.Cr[i] = rnd(), rnd()
		}
		for _, cs := range []yuv.ColorSpace{yuv.BT601, yuv.BT709} {
			lut := yuv.NewRGBLUT(cs, yuv.LimitedRange)
			got := scaleYCbCrAreaRGBRows(img, sz[2], sz[3], lut)
			want := scaleYCbCrAreaSerial(img, sz[2], sz[3], nil, yccAreaAvgRGB, lut)
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("size %v cs %v: output differs", sz, cs)
			}
		}
	}
}

func BenchmarkScaleYCbCrAreaRGB1080To320(b *testing.B) {
	img := image.NewYCbCr(image.Rect(0, 0, 1920, 1080), image.YCbCrSubsampleRatio420)
	for i := range img.Y {
		img.Y[i] = byte(i * 7)
	}
	for i := range img.Cb {
		img.Cb[i], img.Cr[i] = byte(i*3), byte(i*5)
	}
	lut := yuv.NewRGBLUT(yuv.BT709, yuv.LimitedRange)
	b.Run("tiles", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			scaleYCbCrAreaSerial(img, 320, 180, nil, yccAreaAvgRGB, lut)
		}
	})
	b.Run("rows", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			scaleYCbCrAreaRGBRows(img, 320, 180, lut)
		}
	})
}
