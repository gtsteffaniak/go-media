package matroska

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"io"

	gvp8 "github.com/gen2brain/vpx/vp8"
	"github.com/gtsteffaniak/go-media/internal/vp8decode"
	"github.com/gtsteffaniak/go-media/internal/vp9decode"
)

// videoFrame is one demuxed video sample with a Matroska timecode in milliseconds.
type videoFrame struct {
	timestampMs int64
	keyframe    bool
	payload     []byte
}

// ThumbnailParams configures VP8/MJPEG thumbnail decode and downscale.
type ThumbnailParams struct {
	SeekPercent float64
	MaxWidth    int
	MaxHeight   int
	// Scaler is internal/thumb.ScalerMode as int (0 = default high).
	Scaler int
	// Exact decodes VP8/VP9 forward from the keyframe to the frame at the
	// seek time. When false, the keyframe at or before the seek time is used.
	Exact bool
}

// VP8PictureAtSeek demuxes and decodes the VP8 frame at seekPercent without RGB conversion.
// h must come from ProbeHeader on the same reader.
func VP8PictureAtSeek(r io.ReaderAt, size int64, h *Header, seekPercent float64, exact bool) (*gvp8.Picture, bool) {
	if h == nil || !h.HasDuration() {
		return nil, false
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(h.durationMs()) * seekPercent / 100.0)
	frames, err := collectVP8FramesForSeek(r, size, h.timecodeScale(), targetMs)
	if err != nil || len(frames) == 0 {
		return nil, false
	}
	pic := decodeVP8SeekPicture(frames, targetMs, exact)
	if pic == nil {
		return nil, false
	}
	return pic, true
}

func collectVP8FramesForSeek(r io.ReaderAt, size int64, scale uint64, targetMs int64) ([]videoFrame, error) {
	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, err
	}
	track, _, ok := videoTrackNumber(ix.tracks, "V_VP8")
	if !ok {
		return nil, nil
	}
	return collectSeekFrames(r, ix, track, scale, targetMs, true)
}

// ThumbnailRGBA seeks to seekPercent of the file timeline and decodes a VP8 or MJPEG frame.
// H.264 and other codecs return (nil, false).
// h must come from ProbeHeader on the same reader.
func ThumbnailRGBA(r io.ReaderAt, size int64, h *Header, p ThumbnailParams) (*image.RGBA, bool) {
	if h == nil || !h.HasDuration() {
		return nil, false
	}
	scale := h.timecodeScale()
	seekPercent := p.SeekPercent
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(h.durationMs()) * seekPercent / 100.0)

	ix, err := indexSegment(r, size)
	if err != nil {
		return nil, false
	}
	track, codec, ok := videoTrackNumber(ix.tracks, "V_VP8", "V_VP9", "V_MJPEG")
	if !ok {
		return nil, false
	}
	chain := codec == "V_VP8" || codec == "V_VP9"
	frames, err := collectSeekFrames(r, ix, track, scale, targetMs, chain)
	if err != nil {
		return nil, false
	}
	if codec == "V_VP8" {
		pic := decodeVP8SeekPicture(frames, targetMs, p.Exact)
		if pic == nil {
			return nil, false
		}
		return vp8decode.PictureRGBAScaled(pic, p.MaxWidth, p.MaxHeight, p.Scaler), true
	}
	if codec == "V_VP9" {
		pic, err := decodeVP9SeekPicture(frames, targetMs, p.Exact)
		if err != nil || pic == nil {
			return nil, false
		}
		return vp9decode.PictureRGBAScaled(pic, p.MaxWidth, p.MaxHeight, p.Scaler), true
	}
	return decodeMJPEGSeek(frames, targetMs)
}

// timecodeTicksToMs converts Matroska cluster timecode ticks to milliseconds.
func timecodeTicksToMs(ticks int64, timecodeScale uint64) int64 {
	return ticks * int64(timecodeScale) / 1_000_000
}

func decodeVP8SeekPicture(frames []videoFrame, targetMs int64, exact bool) *gvp8.Picture {
	if len(frames) == 0 {
		return nil
	}
	start := 0
	for i, f := range frames {
		if f.keyframe && f.timestampMs <= targetMs {
			start = i
		}
	}
	if !exact {
		// Keyframe mode: stop at the keyframe instead of decoding forward.
		targetMs = frames[start].timestampMs
	}
	var dec gvp8.Decoder
	var last *gvp8.Picture
	for i := start; i < len(frames); i++ {
		pic, err := vp8decode.DecodeFramePicture(&dec, frames[i].payload)
		if err != nil {
			return nil
		}
		if pic != nil {
			last = pic
		}
		if frames[i].timestampMs >= targetMs && last != nil {
			return last
		}
	}
	return last
}

func decodeVP8Seek(frames []videoFrame, targetMs int64, maxW, maxH int, scaler int) (*image.RGBA, bool) {
	last := decodeVP8SeekPicture(frames, targetMs, true)
	if last == nil {
		return nil, false
	}
	return vp8decode.PictureRGBAScaled(last, maxW, maxH, scaler), true
}

func decodeMJPEGSeek(frames []videoFrame, targetMs int64) (*image.RGBA, bool) {
	if len(frames) == 0 {
		return nil, false
	}
	var pick []byte
	for _, f := range frames {
		if f.timestampMs <= targetMs {
			pick = f.payload
		}
	}
	if len(pick) == 0 {
		pick = frames[0].payload
	}
	if len(pick) == 0 {
		return nil, false
	}
	img, _, err := image.Decode(bytes.NewReader(pick))
	if err != nil {
		return nil, false
	}
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba, true
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out, true
}
