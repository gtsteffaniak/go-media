package vc1

import "github.com/gtsteffaniak/go-media/internal/errs"

// DecodeKeyframe decodes one WMV3/WVC1 intra frame. config is sequence-layer
// extradata; sample is the compressed frame; width and height are the coded size.
func DecodeKeyframe(config, sample []byte, width, height int) (*Picture, error) {
	if len(sample) == 0 || width <= 0 || height <= 0 {
		return nil, errs.Corrupt
	}
	return decodeIntra(config, sample, width, height)
}
