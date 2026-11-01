package matroska

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/av1decode"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

const CodecAV1 = "V_AV1"

// AV1ThumbYCbCrAtSeek decodes the AV1 keyframe at or before seekPercent.
func AV1ThumbYCbCrAtSeek(r io.ReaderAt, size int64, h *Header, seekPercent float64) (*h264.ThumbYCbCr, error) {
	if h == nil || !h.HasDuration() {
		return nil, errs.Unsup("thumbnail", "matroska: no duration, seek impossible")
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(h.durationMs()) * seekPercent / 100.0)
	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, av1ThumbErr(err)
	}
	track, _, ok := videoTrackNumber(ix.tracks, CodecAV1)
	if !ok {
		return nil, nil
	}
	var av1C []byte
	for i, t := range ix.tracks {
		n := t.TrackNumber
		if n == 0 {
			n = uint64(i + 1)
		}
		if n == track {
			av1C = t.CodecPrivate
			break
		}
	}
	pick, _, err := keyframeAtSeek(r, ix, track, h.timecodeScale(), targetMs, "AV1")
	if err != nil {
		return nil, av1ThumbErr(err)
	}
	if pick == nil {
		return nil, errs.Unsup("thumbnail", "matroska: no AV1 keyframe found")
	}
	return av1decode.DecodeSampleOnce(av1C, pick)
}

func av1ThumbErr(err error) error {
	return mpeg4ThumbErr(err)
}
