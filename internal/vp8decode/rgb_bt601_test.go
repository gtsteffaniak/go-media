package vp8decode

import (
	"image"
	"testing"

	gvp8 "github.com/gen2brain/vpx/vp8"
)

// TestYUV420BT601Limited checks the VP8 conversion uses ffmpeg's default for
// VP8 (BT.601 matrix, limited range): pure blue as libvpx encodes it maps
// back to blue, not to the BT.709 tint the old path produced.
func TestYUV420BT601Limited(t *testing.T) {
	cases := []struct {
		y, u, v uint8
		want    [3]uint8
	}{
		{16, 128, 128, [3]uint8{0, 0, 0}},
		{235, 128, 128, [3]uint8{255, 255, 255}},
		{41, 240, 110, [3]uint8{0, 0, 255}}, // BT.601 limited blue
		{81, 90, 240, [3]uint8{255, 0, 0}},  // BT.601 limited red
	}
	for _, c := range cases {
		p := &gvp8.Picture{Width: 2, Height: 2, YStride: 2, UVStride: 1,
			Y: []uint8{c.y, c.y, c.y, c.y}, U: []uint8{c.u}, V: []uint8{c.v}}
		dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
		yuv420BT601LimitedToRGBA(dst, p)
		for i := 0; i < 3; i++ {
			d := int(dst.Pix[i]) - int(c.want[i])
			if d < -2 || d > 2 {
				t.Errorf("yuv(%d,%d,%d): got %v want %v", c.y, c.u, c.v, dst.Pix[:3], c.want)
				break
			}
		}
	}
}
