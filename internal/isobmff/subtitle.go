package isobmff

import (
	"encoding/binary"
	"io"
	"strings"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/detect"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/subtitle"
)

// SubtitleTracks lists embedded subtitle streams in MP4/MOV.
func SubtitleTracks(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	return subtitleStreams(r, size)
}

func subtitleStreams(r io.ReaderAt, size int64) ([]detect.Stream, error) {
	m, err := OpenFileMeta(r, size)
	if err != nil {
		return nil, err
	}
	return m.SubtitleStreams(), nil
}

// ExtractSubtitleWebVTT returns WebVTT for an embedded MP4 subtitle track index.
func ExtractSubtitleWebVTT(r io.ReaderAt, size int64, streamIndex int) (string, error) {
	metas, err := listTrakMeta(r, size)
	if err != nil {
		return "", err
	}
	if streamIndex < 0 || streamIndex >= len(metas) {
		return "", errs.Unsupported
	}
	meta := metas[streamIndex]
	if !isSubtitleHandler(meta.HandlerType) {
		return "", errs.Unsup("subtitle", "not a subtitle track")
	}
	codec := subtitleCodecFromFourCC(meta.SampleFourCC)
	switch codec {
	case "wvtt":
		return extractWVTT(r, size, streamIndex)
	case "stpp":
		return extractSTPP(r, size, streamIndex)
	case "tx3g":
		return extractTX3G(r, size, streamIndex)
	default:
		return "", errs.Unsup("subtitle extract", codec)
	}
}

// extractWVTT converts an ISO/IEC 14496-30 WebVTT track (wvtt). ffmpeg 7.1
// cannot read or write these, so there is no ffmpeg output to match; the
// cues are produced the way ffmpeg handles every other WebVTT source: each
// vttc box's payl text becomes a cue, timed by the sample's stts time and
// duration (the edit list is not applied, as for tx3g), identifiers (iden) and
// settings (sttg) are dropped, and the text goes through the same WebVTT
// decoder and writer as sidecar .vtt files. vtte samples mark gaps.
func extractWVTT(r io.ReaderAt, size int64, trackIndex int) (string, error) {
	refs, err := SampleRefs(r, size, trackIndex)
	if err != nil {
		return "", err
	}
	tt, err := readTextTrackTiming(r, size, trackIndex)
	if err != nil {
		return "", err
	}
	var events []subtitle.Event
	var t int64
	for i, ref := range refs {
		var delta int64
		if i < len(tt.deltas) {
			delta = tt.deltas[i]
		}
		start := t
		t += delta
		if ref.Size <= 0 {
			continue
		}
		payload, err := ioat.ReadAll(r, size, ref.Offset, ref.Size)
		if err != nil {
			return "", err
		}
		startMs := rescaleRound(rescaleRound(start*1_000_000, tt.timescale), 1000)
		durMs := rescaleRound(delta*1000, tt.timescale)
		for _, text := range wvttCueTexts(payload) {
			events = append(events, subtitle.Event{
				Start:    startMs,
				Duration: durMs,
				Dialog:   subtitle.DefaultDialog(len(events), subtitle.WebVTTToASS(text, startMs, startMs+durMs)),
			})
		}
	}
	if len(events) == 0 {
		return "", errs.Unsup("subtitle", "no wvtt samples")
	}
	return subtitle.WriteWebVTT("", events), nil
}

// wvttCueTexts returns the payl text of every vttc box in one wvtt sample,
// with trailing CR/LF trimmed; empty payloads are skipped as ffmpeg skips
// empty WebVTT cues.
func wvttCueTexts(sample []byte) []string {
	var out []string
	eachBox(sample, func(typ string, body []byte) {
		if typ != "vttc" {
			return
		}
		eachBox(body, func(typ string, body []byte) {
			if typ != "payl" {
				return
			}
			if text := strings.TrimRight(string(body), "\r\n\x00"); text != "" {
				out = append(out, text)
			}
		})
	})
	return out
}

func extractSTPP(r io.ReaderAt, size int64, trackIndex int) (string, error) {
	refs, err := SampleRefs(r, size, trackIndex)
	if err != nil {
		return "", err
	}
	var xmlParts []string
	for _, ref := range refs {
		payload, err := ioat.ReadAll(r, size, ref.Offset, ref.Size)
		if err != nil {
			return "", err
		}
		xmlParts = append(xmlParts, string(payload))
	}
	if len(xmlParts) == 0 {
		return "", errs.Unsup("subtitle", "no stpp samples")
	}
	return subtitle.TTMLOrXMLToWebVTT(strings.Join(xmlParts, "\n"))
}

// extractTX3G converts a 3GPP timed text (tx3g, ffmpeg's mov_text) track the
// way "ffmpeg -i in.mp4 -map 0:s:N out.vtt" does (ffmpeg 7.1): samples are
// decoded with the track's default style (subtitle.MovText), each sample's
// time comes from stts in the media timescale, its duration is the stts
// delta, and the 2-byte empty samples
// that fill gaps produce no cue. Times are rounded to milliseconds as the
// ffmpeg CLI rounds them.
//
// The edit list is not applied. For the edit lists ffmpeg writes (one edit
// at media_time 0) and for a leading empty edit, that is exactly ffmpeg's
// output: the CLI subtracts the start_time the empty edit creates. For a
// non-zero media_time ffmpeg's mov demuxer re-bases the track on its first
// kept sample, which go-media intentionally does not reproduce.
func extractTX3G(r io.ReaderAt, size int64, trackIndex int) (string, error) {
	refs, err := SampleRefs(r, size, trackIndex)
	if err != nil {
		return "", err
	}
	tt, err := readTextTrackTiming(r, size, trackIndex)
	if err != nil {
		return "", err
	}
	dec := subtitle.NewMovText(tt.entry)
	var events []subtitle.Event
	var t int64
	for i, ref := range refs {
		var delta int64
		if i < len(tt.deltas) {
			delta = tt.deltas[i]
		}
		start := t
		t += delta
		if ref.Size <= 0 {
			continue
		}
		payload, err := ioat.ReadAll(r, size, ref.Offset, ref.Size)
		if err != nil {
			return "", err
		}
		text, ok := dec.Decode(payload)
		if !ok {
			continue
		}
		events = append(events, subtitle.Event{
			Start:    rescaleRound(rescaleRound(start*1_000_000, tt.timescale), 1000),
			Duration: rescaleRound(delta*1000, tt.timescale),
			Dialog:   subtitle.DefaultDialog(len(events), text),
		})
	}
	if len(events) == 0 {
		return "", errs.Unsup("subtitle", "no tx3g samples")
	}
	return subtitle.WriteWebVTT(dec.Header(), events), nil
}

// textTrackTiming is what extractTX3G and extractWVTT need from a trak besides sample offsets.
type textTrackTiming struct {
	timescale int64   // mdhd timescale
	deltas    []int64 // per-sample stts durations in media ticks
	entry     []byte  // first sample entry after size, type, reserved and data reference index
}

// readTextTrackTiming reads mdhd, stts and the first
// stsd entry of the trak at trackIndex.
func readTextTrackTiming(r io.ReaderAt, size int64, trackIndex int) (textTrackTiming, error) {
	var tt textTrackTiming
	rs := ioat.New(r, size)
	traks, err := mp4.ExtractBox(rs, nil, mp4.BoxPath{mp4.BoxTypeMoov(), mp4.BoxTypeTrak()})
	if err != nil {
		return tt, err
	}
	if trackIndex >= len(traks) {
		return tt, errs.Unsupported
	}
	stbl := []mp4.BoxType{mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl()}
	boxes, err := mp4.ExtractBoxesWithPayload(rs, traks[trackIndex], []mp4.BoxPath{
		{mp4.BoxTypeMdia(), mp4.BoxTypeMdhd()},
		append(append(mp4.BoxPath{}, stbl...), mp4.BoxTypeStts()),
	})
	if err != nil {
		return tt, err
	}
	tt.timescale = 1000
	for _, b := range boxes {
		switch p := b.Payload.(type) {
		case *mp4.Mdhd:
			if p.Timescale != 0 {
				tt.timescale = int64(p.Timescale)
			}
		case *mp4.Stts:
			for _, e := range p.Entries {
				if uint64(len(tt.deltas))+uint64(e.SampleCount) > maxTX3GSamples {
					return tt, errs.Unsup("subtitle", "tx3g track has too many samples")
				}
				for n := uint32(0); n < e.SampleCount; n++ {
					tt.deltas = append(tt.deltas, int64(e.SampleDelta))
				}
			}
		}
	}
	stsds, err := mp4.ExtractBox(rs, traks[trackIndex], append(append(mp4.BoxPath{}, stbl...), mp4.BoxTypeStsd()))
	if err != nil {
		return tt, err
	}
	if len(stsds) > 0 && stsds[0].Size <= 1<<16 {
		raw, err := ioat.ReadAll(r, size, int64(stsds[0].Offset+stsds[0].HeaderSize), int64(stsds[0].Size-stsds[0].HeaderSize))
		if err != nil {
			return tt, err
		}
		// FullBox header (4), entry count (4), then the first entry.
		if len(raw) >= 8+16 {
			e := raw[8:]
			n := int(binary.BigEndian.Uint32(e))
			if n >= 16 && n <= len(e) {
				tt.entry = e[16:n]
			}
		}
	}
	return tt, nil
}

// maxTX3GSamples bounds the expanded stts of a tx3g track.
const maxTX3GSamples = 1 << 20

// rescaleRound returns a/d rounded half away from zero, as av_rescale does.
func rescaleRound(a, d int64) int64 {
	if a < 0 {
		return -((-a + d/2) / d)
	}
	return (a + d/2) / d
}
