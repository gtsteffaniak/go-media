package thumb

import (
	"image"
	"image/color"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
)

func TestOrientationFromHEIF(t *testing.T) {
	cases := []struct {
		in   heifthumb.Transform
		want orientation
	}{
		{heifthumb.Transform{}, orientation{}},
		{heifthumb.Transform{Rotation: 90}, orientation{rot: 90}},
		{heifthumb.Transform{Mirror: true, MirrorAxis: 1}, orientation{hflip: true}},
		{heifthumb.Transform{Mirror: true, MirrorAxis: 0}, orientation{rot: 180, hflip: true}},
		{heifthumb.Transform{Rotation: 270, Mirror: true, MirrorAxis: 0}, orientation{rot: 90, hflip: true}},
	}
	for _, c := range cases {
		if got := orientationFromHEIF(c.in); got != c.want {
			t.Errorf("%+v: got %+v want %+v", c.in, got, c.want)
		}
	}
}

func TestHFlipImage(t *testing.T) {
	ycc := image.NewYCbCr(image.Rect(0, 0, 4, 2), image.YCbCrSubsampleRatio420)
	copy(ycc.Y, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	ycc.Cb[0], ycc.Cb[1] = 10, 20
	out := hflipImage(ycc).(*image.YCbCr)
	if out.Y[0] != 4 || out.Y[4] != 8 || out.Cb[0] != 20 {
		t.Fatalf("ycbcr flip wrong: %v %v", out.Y, out.Cb)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, 3, 1))
	rgba.Set(0, 0, color.RGBA{255, 0, 0, 255})
	f := hflipImage(rgba)
	if r, _, _, _ := f.At(2, 0).RGBA(); r != 0xffff {
		t.Fatal("rgba flip wrong")
	}
}
