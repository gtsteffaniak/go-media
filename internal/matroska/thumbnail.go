package matroska

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"io"

	"github.com/at-wat/ebml-go/mkvcore"
	gvp8 "github.com/gen2brain/vpx/vp8"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/vp8decode"
)

// videoFrame is one demuxed video sample with a Matroska timecode in milliseconds.
type videoFrame struct {
	timestampMs int64
	keyframe    bool
	payload     []byte
}

// ThumbnailRGBA seeks to seekPercent of the file timeline and decodes a VP8 or MJPEG frame.
// H.264 and other codecs return (nil, false).
func ThumbnailRGBA(r io.ReaderAt, size int64, seekPercent float64) (*image.RGBA, bool) {
	head, err := readHead(r, size)
	if err != nil || head.Segment.Info.Duration <= 0 {
		return nil, false
	}
	scale := head.Segment.Info.TimecodeScale
	if scale == 0 {
		scale = 1_000_000
	}
	durationMs := int64(head.Segment.Info.Duration * float64(scale) / 1e6)
	if durationMs <= 0 {
		durationMs = 1
	}
	if seekPercent <= 0 {
		seekPercent = 10
	}
	targetMs := int64(float64(durationMs) * seekPercent / 100.0)

	reader := io.NewSectionReader(ioat.New(r, size), 0, size)
	readers, err := mkvcore.NewSimpleBlockReader(reader)
	if err != nil {
		return nil, false
	}

	var vp8Track mkvcore.BlockReadCloserWithTrackEntry
	var mjpegTrack mkvcore.BlockReadCloserWithTrackEntry
	for _, br := range readers {
		switch br.TrackEntry().CodecID {
		case "V_VP8":
			vp8Track = br
		case "V_MJPEG":
			mjpegTrack = br
		}
	}

	defer closeBlockReaders(readers)

	if vp8Track != nil {
		frames := collectVideoFrames(vp8Track)
		if img, ok := decodeVP8Seek(frames, targetMs); ok {
			return img, true
		}
	}
	if mjpegTrack != nil {
		frames := collectVideoFrames(mjpegTrack)
		if img, ok := decodeMJPEGSeek(frames, targetMs); ok {
			return img, true
		}
	}
	return nil, false
}

func closeBlockReaders(readers []mkvcore.BlockReadCloserWithTrackEntry) {
	for _, br := range readers {
		if br != nil {
			br.Close()
		}
	}
}

func collectVideoFrames(br mkvcore.BlockReadCloserWithTrackEntry) []videoFrame {
	var out []videoFrame
	for {
		b, key, ts, err := br.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(b) == 0 {
			break
		}
		out = append(out, videoFrame{
			timestampMs: ts,
			keyframe:    key,
			payload:     append([]byte(nil), b...),
		})
	}
	return out
}

func decodeVP8Seek(frames []videoFrame, targetMs int64) (*image.RGBA, bool) {
	if len(frames) == 0 {
		return nil, false
	}
	start := 0
	for i, f := range frames {
		if f.keyframe && f.timestampMs <= targetMs {
			start = i
		}
	}
	var dec gvp8.Decoder
	var last *image.RGBA
	for i := start; i < len(frames); i++ {
		img, err := vp8decode.DecodeFrame(&dec, frames[i].payload)
		if err != nil {
			return nil, false
		}
		if img != nil {
			last = img
		}
		if frames[i].timestampMs >= targetMs && last != nil {
			return last, true
		}
	}
	if last != nil {
		return last, true
	}
	return nil, false
}

func decodeMJPEGSeek(frames []videoFrame, targetMs int64) (*image.RGBA, bool) {
	var pick []byte
	for _, f := range frames {
		if f.timestampMs <= targetMs {
			pick = f.payload
		}
	}
	if len(pick) == 0 {
		pick = frames[0].payload
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
