package isobmff

import (
	"bytes"
	"encoding/binary"
	"fmt"
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

func extractWVTT(r io.ReaderAt, size int64, trackIndex int) (string, error) {
	refs, err := SampleRefs(r, size, trackIndex)
	if err != nil {
		return "", err
	}
	var cues []string
	for _, ref := range refs {
		if ref.Size <= 0 {
			continue
		}
		payload, err := ioat.ReadAll(r, size, ref.Offset, ref.Size)
		if err != nil {
			return "", err
		}
		cue, err := parseWVTSample(payload)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(cue) != "" {
			cues = append(cues, cue)
		}
	}
	if len(cues) == 0 {
		return "", errs.Unsup("subtitle", "no wvtt samples")
	}
	return "WEBVTT\n\n" + strings.Join(cues, "\n\n") + "\n", nil
}

func parseWVTSample(payload []byte) (string, error) {
	s := strings.TrimSpace(string(payload))
	if strings.Contains(s, "-->") {
		return s, nil
	}
	var start, body string
	_, err := mp4.ReadBoxStructure(bytes.NewReader(payload), func(h *mp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case mp4.BoxTypeCtim(), mp4.BoxTypePayl():
			r := bytes.NewReader(payload)
			if _, err := h.BoxInfo.SeekToPayload(r); err != nil {
				return nil, err
			}
			data := make([]byte, h.BoxInfo.Size-h.BoxInfo.HeaderSize)
			if _, err := io.ReadFull(r, data); err != nil {
				return nil, err
			}
			txt := strings.TrimRight(string(data), "\x00")
			if h.BoxInfo.Type == mp4.BoxTypeCtim() {
				start = txt
			} else {
				body = txt
			}
		}
		if h.BoxInfo.Type != mp4.BoxTypeAny() {
			_, err := h.Expand()
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return "", err
	}
	if start != "" && body != "" {
		return fmt.Sprintf("%s --> %s\n%s", start, start, body), nil
	}
	return "", errs.Unsup("subtitle", "empty wvtt sample")
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

func extractTX3G(r io.ReaderAt, size int64, trackIndex int) (string, error) {
	refs, err := SampleRefs(r, size, trackIndex)
	if err != nil {
		return "", err
	}
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return "", err
	}
	if trackIndex >= len(info.Tracks) {
		return "", errs.Unsupported
	}
	timescale := info.Tracks[trackIndex].Timescale
	if timescale == 0 {
		timescale = 1000
	}
	var cues []string
	var t uint64
	for _, ref := range refs {
		payload, err := ioat.ReadAll(r, size, ref.Offset, ref.Size)
		if err != nil {
			return "", err
		}
		text, dur, err := parseTX3GSample(payload)
		if err != nil {
			return "", err
		}
		if text == "" {
			continue
		}
		start := tx3gTime(t, timescale)
		end := tx3gTime(t+dur, timescale)
		cues = append(cues, fmt.Sprintf("%s --> %s\n%s", start, end, text))
		t += dur
	}
	if len(cues) == 0 {
		return "", errs.Unsup("subtitle", "no tx3g samples")
	}
	return "WEBVTT\n\n" + strings.Join(cues, "\n\n") + "\n", nil
}

func parseTX3GSample(payload []byte) (text string, duration uint64, err error) {
	if len(payload) < 2 {
		return "", 0, errs.Unsup("subtitle", "tx3g sample too short")
	}
	off := 0
	if len(payload) >= 2 && payload[0] == 0 && payload[1] == 0 {
		off = 2
	}
	raw := payload[off:]
	if len(raw) >= 2 && raw[0] == 0xfe && raw[1] == 0xff {
		text = decodeUTF16BE(raw[2:])
	} else {
		text = strings.TrimRight(string(raw), "\x00")
	}
	text = strings.TrimSpace(text)
	duration = 2000
	return text, duration, nil
}

func decodeUTF16BE(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	var runes []rune
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(binary.BigEndian.Uint16(b[i : i+2]))
		if r != 0 {
			runes = append(runes, r)
		}
	}
	return string(runes)
}

func tx3gTime(ticks uint64, timescale uint32) string {
	ms := ticks * 1000 / uint64(timescale)
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
