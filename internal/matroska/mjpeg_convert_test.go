package matroska

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// TestYCbCrToRGBAMatchesColorModel proves the direct YCbCr conversion in
// toRGBA equals the color-model conversion for every input triple.
func TestYCbCrToRGBAMatchesColorModel(t *testing.T) {
	for y := 0; y < 256; y++ {
		for cb := 0; cb < 256; cb++ {
			for cr := 0; cr < 256; cr++ {
				r, g, b := color.YCbCrToRGB(uint8(y), uint8(cb), uint8(cr))
				c := color.RGBAModel.Convert(color.YCbCr{Y: uint8(y), Cb: uint8(cb), Cr: uint8(cr)}).(color.RGBA)
				if r != c.R || g != c.G || b != c.B {
					t.Fatalf("y=%d cb=%d cr=%d: direct (%d,%d,%d), model (%d,%d,%d)", y, cb, cr, r, g, b, c.R, c.G, c.B)
				}
			}
		}
	}
}

// TestToRGBAMatchesGeneric checks the plane fast paths against the generic
// per-pixel conversion on random images with odd bounds and all subsampling.
func TestToRGBAMatchesGeneric(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	ratios := []image.YCbCrSubsampleRatio{image.YCbCrSubsampleRatio444, image.YCbCrSubsampleRatio422, image.YCbCrSubsampleRatio420, image.YCbCrSubsampleRatio440}
	bounds := []image.Rectangle{image.Rect(0, 0, 37, 23), image.Rect(5, -3, 42, 19)}
	for _, b := range bounds {
		for _, ratio := range ratios {
			img := image.NewYCbCr(b, ratio)
			rng.Read(img.Y)
			rng.Read(img.Cb)
			rng.Read(img.Cr)
			assertSameRGBA(t, img, toRGBA(img))
		}
		g := image.NewGray(b)
		rng.Read(g.Pix)
		assertSameRGBA(t, g, toRGBA(g))
	}
}

func assertSameRGBA(t *testing.T, img image.Image, got *image.RGBA) {
	t.Helper()
	b := img.Bounds()
	want := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			want.Set(x, y, img.At(x, y))
		}
	}
	if got.Rect != want.Rect || string(got.Pix) != string(want.Pix) {
		t.Fatalf("%T %v: fast path differs from generic conversion", img, b)
	}
}
