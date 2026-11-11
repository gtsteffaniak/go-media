package av1decode

import (
	"testing"

	"github.com/gen2brain/gav1d/av1"
)

// TestPictureThumbYCbCrLayouts checks 4:4:4 and 4:2:2 chroma is averaged to
// 4:2:0 across the full width, and that 10-bit samples round to 8 bits.
func TestPictureThumbYCbCrLayouts(t *testing.T) {
	const w, h = 4, 2
	y := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	// 4:4:4: left half chroma 100, right half 200.
	c444 := []byte{100, 100, 200, 200, 100, 100, 200, 200}
	p := &av1.Picture{Width: w, Height: h, Layout: av1.LayoutI444, BitDepth: 8,
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
		y16[2*i], y16[2*i+1] = byte(v), byte(v>>8)
	}
	c16 := []byte{0, 2, 0, 2} // two samples of 512
	p = &av1.Picture{Width: w, Height: h, Layout: av1.LayoutI420, BitDepth: 10,
		Data: [3][]byte{y16, c16, c16}, Stride: [3]int{w * 2, 4, 4}}
	th = PictureThumbYCbCr(p)
	if th.YCbCr.Y[0] != 255 || th.YCbCr.Y[1] != 129 || th.YCbCr.Cb[0] != 128 {
		t.Fatalf("10-bit Y=%v Cb=%v", th.YCbCr.Y[:2], th.YCbCr.Cb[:1])
	}
}
