package vp8decode

import (
	"testing"

	gvp8 "github.com/gen2brain/vpx/vp8"
)

func TestHalve420OddDimensions(t *testing.T) {
	p := &gvp8.Picture{
		Width: 800, Height: 450,
		YStride: 800, UVStride: 400,
		Y: make([]byte, 800*450),
		U: make([]byte, 400*225),
		V: make([]byte, 400*225),
	}
	for i := range p.Y {
		p.Y[i] = byte(i % 251)
	}
	for i := range p.U {
		p.U[i] = byte((i * 3) % 251)
		p.V[i] = byte((i * 7) % 251)
	}
	out := halve420(p)
	if out.Width != 400 || out.Height != 225 {
		t.Fatalf("size %dx%d", out.Width, out.Height)
	}
	ncw := (out.Width + 1) / 2
	nch := (out.Height + 1) / 2
	if len(out.U) < ncw*nch || len(out.V) < ncw*nch {
		t.Fatalf("chroma too small u=%d v=%d need %d", len(out.U), len(out.V), ncw*nch)
	}
	_ = PictureRGBAScaled(out, 256, 144, 1)
}
