package av1decode

import (
	"image"
	"testing"

	"github.com/gen2brain/gav1d/avif"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

func TestAVIFColorMapping(t *testing.T) {
	cases := []struct {
		m    uint16
		full bool
		cs   yuv.ColorSpace
		rng  yuv.Range
	}{
		{0, false, yuv.BT601, yuv.LimitedRange},
		{2, true, yuv.BT601, yuv.FullRange},
		{1, false, yuv.BT709, yuv.LimitedRange},
		{9, true, yuv.BT2020, yuv.FullRange},
		{6, false, yuv.BT601, yuv.LimitedRange},
	}
	for _, c := range cases {
		cs, rng := avifColor(avif.ColorInfo{Matrix: c.m, FullRange: c.full})
		if cs != c.cs || rng != c.rng {
			t.Errorf("matrix %d full %v: got %v/%v want %v/%v", c.m, c.full, cs, rng, c.cs, c.rng)
		}
	}
}

func TestAVIFToThumbKeepsMatrixFor444(t *testing.T) {
	src := image.NewYCbCr(image.Rect(0, 0, 5, 3), image.YCbCrSubsampleRatio444)
	for i := range src.Y {
		src.Y[i] = 16
		src.Cb[i] = byte(i)
		src.Cr[i] = byte(200 - i)
	}
	th := avifToThumb(src, avif.ColorInfo{Matrix: 1})
	if th.CS != yuv.BT709 || th.RNG != yuv.LimitedRange {
		t.Fatalf("got %v/%v", th.CS, th.RNG)
	}
	if th.YCbCr.SubsampleRatio != image.YCbCrSubsampleRatio420 || th.YCbCr.Rect.Dx() != 5 || th.YCbCr.Rect.Dy() != 3 {
		t.Fatalf("bad layout %v %v", th.YCbCr.SubsampleRatio, th.YCbCr.Rect)
	}
	if th.YCbCr.Y[0] != 16 {
		t.Fatalf("luma changed: %d", th.YCbCr.Y[0])
	}
	// Block (2,2) -> source pixel (4,2).
	si := src.COffset(4, 2)
	di := th.YCbCr.COffset(4, 2)
	if th.YCbCr.Cb[di] != src.Cb[si] || th.YCbCr.Cr[di] != src.Cr[si] {
		t.Fatalf("chroma not taken from block corner")
	}
}

func TestAVIFToThumbGray(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 3, 3))
	for i := range src.Pix {
		src.Pix[i] = 16
	}
	th := avifToThumb(src, avif.ColorInfo{})
	if th.RNG != yuv.LimitedRange || th.YCbCr.Y[0] != 16 || th.YCbCr.Cb[0] != 128 || th.YCbCr.Cr[0] != 128 {
		t.Fatalf("gray: rng %v y %d cb %d cr %d", th.RNG, th.YCbCr.Y[0], th.YCbCr.Cb[0], th.YCbCr.Cr[0])
	}
}
