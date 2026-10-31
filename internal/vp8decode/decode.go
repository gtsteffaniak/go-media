// Package vp8decode decodes VP8 video frames (key and inter) for thumbnails.
//
// VP8 samples are BT.601 limited range (RFC 6386 §9.2, color_space 0); RGB
// conversion here uses that matrix and range, matching ffmpeg's vp8 decoder.
//
// Inter-frame decoding uses github.com/gen2brain/vpx/vp8 (libwebp-derived, BSD-style).
package vp8decode

import (
	"image"

	gvp8 "github.com/gen2brain/vpx/vp8"
)

// DecodeFramePicture decodes one VP8 frame and returns the YUV picture (nil for invisible frames).
func DecodeFramePicture(dec *gvp8.Decoder, frame []byte) (*gvp8.Picture, error) {
	if dec == nil {
		dec = &gvp8.Decoder{}
	}
	pic, err := dec.DecodeFrame(frame)
	if err != nil {
		return nil, err
	}
	return pic, nil
}

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
