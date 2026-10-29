package isobmff

import (
	"io"
	"math"
	"strings"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// trakMeta holds per-track handler and sample-entry metadata from moov.
type trakMeta struct {
	HandlerType   string
	HandlerName   string
	Language      string
	SampleFourCC  string
	Width         int
	Height        int
	AttachedCover bool
	// TrackID and Timescale come from tkhd and mdhd. Timescale is the media
	// timescale used by fragment sample durations.
	TrackID   uint32
	Timescale uint32
	// Rotation is the display rotation in degrees from the tkhd matrix, using
	// ffprobe's "rotation" sign convention: -90 means rotate 90 degrees clockwise.
	Rotation int
}

// listTrakMeta returns one entry per trak in moov order.
func listTrakMeta(r io.ReaderAt, size int64) ([]trakMeta, error) {
	rs := ioat.New(r, size)
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil {
		return nil, err
	}
	out := make([]trakMeta, len(traks))
	for i, trak := range traks {
		meta, err := readTrakMeta(rs, trak)
		if err != nil {
			return nil, err
		}
		out[i] = meta
	}
	return out, nil
}

func readTrakMeta(rs io.ReadSeeker, trak *mp4.BoxInfo) (trakMeta, error) {
	var meta trakMeta
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return meta, err
	}
	stsdPaths := []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeMp4v()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeAvc1()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeMp4a()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeWvtt()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeStpp()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeSbtt()},
	}
	bips, err := mp4.ExtractBoxesWithPayload(rs, trak, append([]mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeHdlr()},
		{mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()},
		{mp4.BoxTypeTkhd()},
	}, stsdPaths...))
	if err != nil {
		return meta, err
	}
	for _, bip := range bips {
		switch bip.Info.Type {
		case mp4.BoxTypeHdlr():
			h := bip.Payload.(*mp4.Hdlr)
			meta.HandlerType = string(h.HandlerType[:])
			meta.HandlerName = strings.TrimSpace(h.Name)
		case mp4.BoxTypeMdhd():
			m := bip.Payload.(*mp4.Mdhd)
			meta.Language = iso639ToString(m.Language)
			meta.Timescale = m.Timescale
		case mp4.BoxTypeTkhd():
			tk := bip.Payload.(*mp4.Tkhd)
			meta.TrackID = tk.TrackID
			meta.Rotation = rotationFromMatrix(tk.Matrix)
			if meta.Width == 0 {
				meta.Width = int(tk.Width >> 16)
			}
			if meta.Height == 0 {
				meta.Height = int(tk.Height >> 16)
			}
		default:
			if meta.SampleFourCC == "" {
				meta.SampleFourCC = bip.Info.Type.String()
				if vse, ok := bip.Payload.(*mp4.VisualSampleEntry); ok {
					meta.Width = int(vse.Width)
					meta.Height = int(vse.Height)
				}
			}
		}
	}
	if meta.SampleFourCC == "" {
		if boxes, err := mp4.ExtractBox(rs, trak, mp4.BoxPath{
			mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.StrToBoxType("tx3g"),
		}); err == nil && len(boxes) > 0 {
			meta.SampleFourCC = "tx3g"
		}
	}
	if meta.SampleFourCC == "" {
		// Sample entries without a payload decoder (AV1, ProRes, PCM, ...) are
		// found with plain box lookups, which do not decode their contents.
		for _, fc := range sampleEntryCandidates(meta.HandlerType) {
			if boxes, err := mp4.ExtractBox(rs, trak, mp4.BoxPath{
				mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.StrToBoxType(fc),
			}); err == nil && len(boxes) > 0 {
				meta.SampleFourCC = fc
				break
			}
		}
	}
	meta.AttachedCover = isAttachedCoverSampleEntry(meta.SampleFourCC, meta.HandlerType)
	return meta, nil
}

func isAttachedCoverSampleEntry(fourCC, handler string) bool {
	if handler != "vide" && handler != "pict" {
		return false
	}
	switch strings.TrimSpace(fourCC) {
	case "jpeg", "jpg ", "png ", "bmp ":
		return true
	default:
		return false
	}
}

func iso639ToString(lang [3]byte) string {
	var b []byte
	for i := 0; i < 3; i++ {
		if lang[i] == 0 {
			break
		}
		b = append(b, lang[i]+0x60)
	}
	return string(b)
}

func isSubtitleHandler(handler string) bool {
	switch handler {
	case "sbtl", "subt", "text":
		return true
	default:
		return false
	}
}

func subtitleCodecFromFourCC(fourCC string) string {
	switch strings.TrimSpace(fourCC) {
	case "wvtt":
		return "wvtt"
	case "stpp":
		return "stpp"
	case "tx3g":
		return "tx3g"
	case "sbtt":
		return "sbtt"
	default:
		return strings.TrimSpace(fourCC)
	}
}

// rotationFromMatrix derives the display rotation from a tkhd transformation
// matrix (16.16 fixed point, a=m[0], b=m[1]). It matches ffmpeg's
// av_display_rotation_get: the result is -atan2(b, a) rounded to a multiple of 90
// and normalized to (-180, 180].
func rotationFromMatrix(m [9]int32) int {
	a := float64(m[0]) / 65536
	b := float64(m[1]) / 65536
	if a == 0 && b == 0 {
		return 0
	}
	deg := -math.Atan2(b, a) * 180 / math.Pi
	r := int(math.Round(deg/90)) * 90
	r = ((r % 360) + 360) % 360
	if r > 180 {
		r -= 360
	}
	return r
}

// sampleEntryCandidates lists the named sample entry types to look for, given the
// track handler.
func sampleEntryCandidates(handler string) []string {
	var out []string
	for _, fc := range detect.MP4SampleEntryFourCCs() {
		if _, ok := detect.MP4VideoCodecName(fc); ok && handler == "vide" {
			out = append(out, fc)
		}
		if _, ok := detect.MP4AudioCodecName(fc); ok && handler == "soun" {
			out = append(out, fc)
		}
	}
	return out
}
