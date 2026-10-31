package vp9decode

import (
	"errors"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-vpx/pkg/govpx"
)

// FrameDecoder decodes raw VP9 packets (Matroska/WebM payloads, possibly superframes).
type FrameDecoder struct {
	dec *govpx.VP9Decoder
}

// NewFrameDecoder constructs a VP9 decoder for thumbnail seeking.
func NewFrameDecoder() (*FrameDecoder, error) {
	d, err := govpx.NewVP9Decoder(govpx.VP9DecoderOptions{})
	if err != nil {
		return nil, err
	}
	return &FrameDecoder{dec: d}, nil
}

// Close releases decoder resources.
func (f *FrameDecoder) Close() error {
	if f == nil || f.dec == nil {
		return nil
	}
	return f.dec.Close()
}

// DecodeFramePicture decodes one VP9 packet and returns the visible I420 frame.
// Invisible frames update reference state and return (nil, nil).
func (f *FrameDecoder) DecodeFramePicture(packet []byte) (*govpx.Image, error) {
	if f == nil || f.dec == nil {
		d, err := NewFrameDecoder()
		if err != nil {
			return nil, err
		}
		f = d
	}
	if err := f.dec.Decode(packet); err != nil {
		return nil, mapDecodeErr(err)
	}
	img, ok := f.dec.NextFrame()
	if !ok {
		return nil, nil
	}
	return &img, nil
}

func mapDecodeErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, govpx.ErrVP9NotImplemented) {
		return errs.UnsupProfile("vp9 keyframe", "profile not supported (VP9 profile 0 8-bit 4:2:0 only)")
	}
	if errors.Is(err, govpx.ErrInvalidData) || errors.Is(err, govpx.ErrInvalidVP9Data) {
		return errs.Corrupt
	}
	return err
}
