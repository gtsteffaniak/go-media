// Package vp8decode decodes VP8 video frames (key and inter) for thumbnails.
//
// Inter-frame decoding uses github.com/gen2brain/vpx/vp8 (libwebp-derived, BSD-style).
package vp8decode

import (
	"image"
	"image/color"

	gvp8 "github.com/gen2brain/vpx/vp8"
)

// DecodeFrame decodes one VP8 frame bitstream (Matroska/WebM payload) into an RGBA image.
// Invisible frames update reference state and return (nil, nil).
func DecodeFrame(dec *gvp8.Decoder, frame []byte) (*image.RGBA, error) {
	if dec == nil {
		dec = &gvp8.Decoder{}
	}
	pic, err := dec.DecodeFrame(frame)
	if err != nil {
		return nil, err
	}
	if pic == nil {
		return nil, nil
	}
	return pictureRGBA(pic), nil
}

// pictureRGBA converts a 4:2:0 VP8 picture into RGBA.
func pictureRGBA(p *gvp8.Picture) *image.RGBA {
	w, h := p.Width, p.Height
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			yy := p.Y[y*p.YStride+x]
			cb := p.U[(y/2)*p.UVStride+(x/2)]
			cr := p.V[(y/2)*p.UVStride+x/2]
			r8, g8, b8 := color.YCbCrToRGB(yy, cb, cr)
			out.SetRGBA(x, y, color.RGBA{R: r8, G: g8, B: b8, A: 255})
		}
	}
	return out
}
