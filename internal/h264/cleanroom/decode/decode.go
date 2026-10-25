package decode

import (
	"errors"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/nal"
)

// Errors for unfinished decoder branches (expected at the current phase).
var (
	ErrNotImplemented = errors.New("h264: decoding of this MB branch is not yet implemented")
	ErrInterSlice     = errors.New("h264: inter-frame slices (P/B) are not yet implemented")
)

// DecodeSlice parses and decodes one slice into frame. I-slices (CAVLC or CABAC)
// with I_PCM, I_16x16 and I_NxN macroblocks are supported. When skipDeblock is true,
// in-loop deblocking is omitted (thumbnail fast preview).
func DecodeSlice(rbsp []byte, nalType nal.Type, nalRefIDC uint8, sps *SPS, pps *PPS, frame *Frame, skipDeblock bool) (*SliceHeader, error) {
	r := bitstream.NewReader(rbsp)
	h, err := readSliceHeader(r, nalType, nalRefIDC, sps, pps)
	if err != nil {
		return nil, err
	}
	if !h.Type.IsI() {
		return h, ErrInterSlice
	}
	sd := acquireSliceDecoder(r, h, sps, pps, frame)
	if err := sd.run(); err != nil {
		releaseSliceDecoder(sd)
		return h, err
	}
	if !skipDeblock {
		sd.deblock() // assumes one slice per frame
	}
	releaseSliceDecoder(sd)
	return h, nil
}
