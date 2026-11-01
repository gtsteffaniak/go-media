package matroska

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

// CodecAVC is the Matroska CodecID for H.264 video. CodecPrivate holds an
// AVCDecoderConfigurationRecord (avcC), as in MP4.
const CodecAVC = "V_MPEG4/ISO/AVC"

// H264Keyframe is the H.264 keyframe block picked for a seek target.
type H264Keyframe struct {
	// Sample is the block payload: length-prefixed NAL units (avcC framing).
	Sample []byte
	// AVCC is the parsed CodecPrivate, or nil when the track has none
	// (parameter sets must then be in band).
	AVCC *mp4.AVCDecoderConfiguration
	// TimeMs is the block timestamp (presentation time) in milliseconds.
	TimeMs int64
}

// H264KeyframeAtSeek returns the H.264 keyframe block at or before
// seekPercent of the file duration (the first keyframe after it when none
// precede it), found through Cues when present. Keyframes are blocks with the
// SimpleBlock keyframe flag (or BlockGroups without a ReferenceBlock); muxers
// set it on IDR and open-GOP recovery pictures alike. It returns (nil, nil)
// when the file has no H.264 video track. h must come from ProbeHeader on the
// same reader.
func H264KeyframeAtSeek(r io.ReaderAt, size int64, h *Header, seekPercent float64) (*H264Keyframe, error) {
	if h == nil || !h.HasDuration() {
		return nil, errs.Unsup("thumbnail", "matroska: no duration, seek impossible")
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(h.durationMs()) * seekPercent / 100.0)
	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, mpeg4ThumbErr(err)
	}
	track, _, ok := videoTrackNumber(ix.tracks, CodecAVC)
	if !ok {
		return nil, nil
	}
	var config []byte
	for i, t := range ix.tracks {
		n := t.TrackNumber
		if n == 0 {
			n = uint64(i + 1)
		}
		if n == track {
			config = t.CodecPrivate
			break
		}
	}
	pick, ms, err := keyframeAtSeek(r, ix, track, h.timecodeScale(), targetMs, "H.264")
	if err != nil {
		return nil, mpeg4ThumbErr(err)
	}
	if pick == nil {
		return nil, errs.Unsup("thumbnail", "matroska: no H.264 keyframe found")
	}
	return &H264Keyframe{Sample: pick, AVCC: h264.ParseAVCConfigRecord(config), TimeMs: ms}, nil
}
