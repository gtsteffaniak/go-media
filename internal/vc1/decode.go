package vc1

import (
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// DecodeKeyframe decodes one WMV3/VC-1 intra frame. config is the BITMAPINFO
// extradata (sequence layer); sample is the compressed frame bytes from ASF.
func DecodeKeyframe(config, sample []byte) (*Picture, error) {
	if len(sample) == 0 {
		return nil, errs.Corrupt
	}
	return decodeIntra(config, sample)
}
