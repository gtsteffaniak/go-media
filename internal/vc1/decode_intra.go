package vc1

import (
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// decodeIntra is implemented in decode_intra.go (generated from SMPTE 421 tables).
func decodeIntra(config, sample []byte) (*Picture, error) {
	_ = config
	_ = sample
	return nil, errs.UnsupProfile("wmv3 keyframe", "VC-1 intra decode not implemented")
}
