package h264

import (
	"github.com/gtsteffaniak/go-media/internal/errs"
)

// DecodeKeyframe decodes an H.264 IDR NAL slice to RGBA.
// Stage 1 (CAVLC) and stage 2 (CABAC) decoders are not implemented yet.
func DecodeKeyframe(nal []byte, width, height int) ([]byte, int, int, error) {
	return nil, 0, 0, errs.Unsup("h264 keyframe", "intra decoder not implemented")
}
