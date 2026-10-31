package matroska

import (
	"github.com/gtsteffaniak/go-media/internal/vp9decode"
	"github.com/gtsteffaniak/govpx"
)

// VP9PictureAtSeek demuxes and decodes the VP9 frame at seekPercent without RGB conversion.
func VP9PictureAtSeek(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, h *Header, seekPercent float64) (*govpx.Image, error) {
	if h == nil || !h.HasDuration() {
		return nil, nil
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(h.durationMs()) * seekPercent / 100.0)
	frames, err := collectVP9FramesForSeek(r, size, h.timecodeScale(), targetMs)
	if err != nil || len(frames) == 0 {
		return nil, err
	}
	return decodeVP9SeekPicture(frames, targetMs)
}

func collectVP9FramesForSeek(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64, scale uint64, targetMs int64) ([]videoFrame, error) {
	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, err
	}
	track, _, ok := videoTrackNumber(ix.tracks, "V_VP9")
	if !ok {
		return nil, nil
	}
	return collectSeekFrames(r, ix, track, scale, targetMs, true)
}

func decodeVP9SeekPicture(frames []videoFrame, targetMs int64) (*govpx.Image, error) {
	if len(frames) == 0 {
		return nil, nil
	}
	start := 0
	for i, f := range frames {
		if f.keyframe && f.timestampMs <= targetMs {
			start = i
		}
	}
	dec, err := vp9decode.NewFrameDecoder()
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	var last *govpx.Image
	for i := start; i < len(frames); i++ {
		pic, err := dec.DecodeFramePicture(frames[i].payload)
		if err != nil {
			return nil, err
		}
		if pic != nil {
			last = pic
		}
		if frames[i].timestampMs >= targetMs && last != nil {
			return last, nil
		}
	}
	return last, nil
}
