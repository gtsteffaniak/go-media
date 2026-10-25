package matroska

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"io"
	"sync"

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

// ThumbnailParams configures VP8/MJPEG thumbnail decode and downscale.
type ThumbnailParams struct {
	SeekPercent float64
	MaxWidth    int
	MaxHeight   int
	// Scaler is internal/thumb.ScalerMode as int (0 = default high).
	Scaler int
}

// VP8PictureAtSeek demuxes and decodes the VP8 frame at seekPercent without RGB conversion.
func VP8PictureAtSeek(r io.ReaderAt, size int64, seekPercent float64) (*gvp8.Picture, bool) {
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
	frames, err := collectVP8FramesForSeek(r, size, targetMs)
	if err != nil || len(frames) == 0 {
		return nil, false
	}
	pic := decodeVP8SeekPicture(frames, targetMs)
	if pic == nil {
		return nil, false
	}
	return pic, true
}

func collectVP8FramesForSeek(r io.ReaderAt, size int64, targetMs int64) ([]videoFrame, error) {
	head, err := readHead(r, size)
	if err != nil {
		return nil, err
	}
	scale := head.Segment.Info.TimecodeScale
	if scale == 0 {
		scale = 1_000_000
	}
	reader := io.NewSectionReader(ioat.New(r, size), 0, size)
	var demuxFatal error
	var demuxMu sync.Mutex
	readers, err := mkvcore.NewSimpleBlockReader(reader, mkvcore.WithOnFatalHandler(func(e error) {
		demuxMu.Lock()
		if demuxFatal == nil {
			demuxFatal = e
		}
		demuxMu.Unlock()
	}))
	if err != nil {
		return nil, err
	}
	var vp8Track mkvcore.BlockReadCloserWithTrackEntry
	for _, br := range readers {
		if br.TrackEntry().CodecID == "V_VP8" {
			vp8Track = br
		}
	}
	closed := map[mkvcore.BlockReadCloserWithTrackEntry]bool{}
	closeTrack := func(br mkvcore.BlockReadCloserWithTrackEntry) {
		if br == nil || closed[br] {
			return
		}
		br.Close()
		closed[br] = true
	}
	for _, br := range readers {
		if br == nil {
			continue
		}
		if br.TrackEntry().CodecID != "V_VP8" {
			closeTrack(br)
		}
	}
	defer func() {
		for _, br := range readers {
			closeTrack(br)
		}
	}()
	if vp8Track == nil {
		return nil, nil
	}
	frames, err := collectVideoFramesForSeek(vp8Track, scale, targetMs, true)
	if getDemuxFatal(&demuxMu, &demuxFatal) != nil {
		return nil, demuxFatal
	}
	return frames, err
}

// ThumbnailRGBA seeks to seekPercent of the file timeline and decodes a VP8 or MJPEG frame.
// H.264 and other codecs return (nil, false).
func ThumbnailRGBA(r io.ReaderAt, size int64, p ThumbnailParams) (*image.RGBA, bool) {
	seekPercent := p.SeekPercent
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
	var demuxFatal error
	var demuxMu sync.Mutex
	readers, err := mkvcore.NewSimpleBlockReader(reader, mkvcore.WithOnFatalHandler(func(e error) {
		demuxMu.Lock()
		if demuxFatal == nil {
			demuxFatal = e
		}
		demuxMu.Unlock()
	}))
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

	closed := map[mkvcore.BlockReadCloserWithTrackEntry]bool{}
	closeTrack := func(br mkvcore.BlockReadCloserWithTrackEntry) {
		if br == nil || closed[br] {
			return
		}
		br.Close()
		closed[br] = true
	}
	for _, br := range readers {
		if br == nil {
			continue
		}
		switch br.TrackEntry().CodecID {
		case "V_VP8", "V_MJPEG":
		default:
			closeTrack(br)
		}
	}
	defer func() {
		for _, br := range readers {
			closeTrack(br)
		}
	}()

	if getDemuxFatal(&demuxMu, &demuxFatal) != nil {
		return nil, false
	}

	if vp8Track != nil {
		frames, err := collectVideoFramesForSeek(vp8Track, scale, targetMs, true)
		if err != nil || getDemuxFatal(&demuxMu, &demuxFatal) != nil {
			return nil, false
		}
		pic := decodeVP8SeekPicture(frames, targetMs)
		if pic == nil {
			return nil, false
		}
		return vp8decode.PictureRGBAScaled(pic, p.MaxWidth, p.MaxHeight, p.Scaler), true
	}
	if mjpegTrack != nil {
		frames, err := collectVideoFramesForSeek(mjpegTrack, scale, targetMs, false)
		if err != nil || getDemuxFatal(&demuxMu, &demuxFatal) != nil {
			return nil, false
		}
		if img, ok := decodeMJPEGSeek(frames, targetMs); ok {
			return img, true
		}
	}
	return nil, false
}

func getDemuxFatal(mu *sync.Mutex, fatal *error) error {
	mu.Lock()
	defer mu.Unlock()
	return *fatal
}

// timecodeTicksToMs converts Matroska cluster timecode ticks to milliseconds.
func timecodeTicksToMs(ticks int64, timecodeScale uint64) int64 {
	return ticks * int64(timecodeScale) / 1_000_000
}

// collectVideoFramesForSeek reads only the frames needed to decode near targetMs.
// For VP8, payloads from the last keyframe at or before target through the first
// frame at or after target are retained. For MJPEG, only the latest frame at or
// before target is kept.
func collectVideoFramesForSeek(br mkvcore.BlockReadCloserWithTrackEntry, timecodeScale uint64, targetMs int64, vp8 bool) ([]videoFrame, error) {
	var out []videoFrame
	var mjpegPick *videoFrame
	for {
		b, key, ts, err := br.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(b) == 0 {
			continue
		}
		tsMs := timecodeTicksToMs(ts, timecodeScale)
		f := videoFrame{
			timestampMs: tsMs,
			keyframe:    key,
			payload:     append([]byte(nil), b...),
		}
		if vp8 {
			if key && tsMs <= targetMs {
				out = nil
			}
			out = append(out, f)
			if tsMs >= targetMs {
				break
			}
			continue
		}
		if tsMs <= targetMs {
			mjpegPick = &f
		} else if mjpegPick != nil {
			break
		}
	}
	if vp8 {
		return out, nil
	}
	if mjpegPick != nil {
		return []videoFrame{*mjpegPick}, nil
	}
	return out, nil
}

func decodeVP8SeekPicture(frames []videoFrame, targetMs int64) *gvp8.Picture {
	if len(frames) == 0 {
		return nil
	}
	start := 0
	for i, f := range frames {
		if f.keyframe && f.timestampMs <= targetMs {
			start = i
		}
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
	last := decodeVP8SeekPicture(frames, targetMs)
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
