package av1decode

import (
	"encoding/binary"
	"testing"

	"github.com/gen2brain/gav1d/av1"

	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
)

// TestPictureThumbYCbCrLayouts checks 4:4:4 and 4:2:2 chroma is averaged to
// 4:2:0 across the full width, and that 10-bit samples round to 8 bits.
func TestPictureThumbYCbCrLayouts(t *testing.T) {
	const w, h = 4, 2
	y := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	// 4:4:4: left half chroma 100, right half 200.
	c444 := []byte{100, 100, 200, 200, 100, 100, 200, 200}
	p := &av1.Picture{Width: w, Height: h, Layout: av1.LayoutI444, BitDepth: 8, MatrixCoeffs: 6,
		Data: [3][]byte{y, c444, c444}, Stride: [3]int{w, w, w}}
	th := PictureThumbYCbCr(p)
	if got := th.YCbCr.Cb; got[0] != 100 || got[1] != 200 {
		t.Fatalf("444 Cb = %v, want [100 200]", got[:2])
	}
	// 4:2:2: two chroma columns, rows 100/200 average to 150.
	c422 := []byte{100, 50, 200, 250}
	p = &av1.Picture{Width: w, Height: h, Layout: av1.LayoutI422, BitDepth: 8,
		Data: [3][]byte{y, c422, c422}, Stride: [3]int{w, 2, 2}}
	th = PictureThumbYCbCr(p)
	if got := th.YCbCr.Cr; got[0] != 150 || got[1] != 150 {
		t.Fatalf("422 Cr = %v, want [150 150]", got[:2])
	}
	// 10-bit 4:2:0: 1022 → 255 (clamped after rounding), 514 → 129.
	y16 := make([]byte, w*h*2)
	for i := 0; i < w*h; i++ {
		v := uint16(1022)
		if i%2 == 1 {
			v = 514
		}
		binary.NativeEndian.PutUint16(y16[2*i:], v)
	}
	c16 := make([]byte, 4)
	binary.NativeEndian.PutUint16(c16[0:], 512)
	binary.NativeEndian.PutUint16(c16[2:], 512)
	p = &av1.Picture{Width: w, Height: h, Layout: av1.LayoutI420, BitDepth: 10,
		Data: [3][]byte{y16, c16, c16}, Stride: [3]int{w * 2, 4, 4}}
	th = PictureThumbYCbCr(p)
	if th.YCbCr.Y[0] != 255 || th.YCbCr.Y[1] != 129 || th.YCbCr.Cb[0] != 128 {
		t.Fatalf("10-bit Y=%v Cb=%v", th.YCbCr.Y[:2], th.YCbCr.Cb[:1])
	}
}

func TestPictureThumbYCbCrIdentity444(t *testing.T) {
	// identity 4:4:4 stores G, B, R in planes 0..2.
	g := []byte{0}
	b := []byte{0}
	r := []byte{255}
	p := &av1.Picture{
		Width: 1, Height: 1, Layout: av1.LayoutI444, BitDepth: 8, MatrixCoeffs: 0,
		Data: [3][]byte{g, b, r}, Stride: [3]int{1, 1, 1},
	}
	th := PictureThumbYCbCr(p)
	if th.CS != yuv.BT601 || th.RNG != yuv.LimitedRange {
		t.Fatalf("tag %v %v", th.CS, th.RNG)
	}
	want := yuv.RGBToYCbCrCS(255, 0, 0, yuv.BT601, yuv.LimitedRange)
	if th.YCbCr.Y[0] != want.Y || th.YCbCr.Cb[0] != want.Cb || th.YCbCr.Cr[0] != want.Cr {
		t.Fatalf("pixel YCbCr=%d,%d,%d want %d,%d,%d",
			th.YCbCr.Y[0], th.YCbCr.Cb[0], th.YCbCr.Cr[0], want.Y, want.Cb, want.Cr)
	}
}
