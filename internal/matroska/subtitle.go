package matroska

import (
	"fmt"
	"io"
	"strings"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/subtitle"
)

// ExtractSubtitleWebVTT converts the Matroska subtitle track at streamIndex to
// WebVTT. Blocks are read synchronously by the bounded cluster walker; blocks
// for other or undeclared tracks are skipped.
func ExtractSubtitleWebVTT(r io.ReaderAt, size int64, streamIndex int) (string, error) {
	head, err := readHead(r, size)
	if err != nil {
		return "", err
	}
	if streamIndex < 0 || streamIndex >= len(head.Segment.Tracks.TrackEntry) {
		return "", errs.Unsupported
	}
	te := head.Segment.Tracks.TrackEntry[streamIndex]
	if te.TrackType != 17 {
		return "", errs.Unsup("subtitle", "not a subtitle track")
	}
	var target uint64 = te.TrackNumber
	if target == 0 {
		target = uint64(streamIndex + 1)
	}
	ix, err := indexSegment(r, size)
	if err != nil {
		return "", err
	}
	if ix.firstCluster < 0 {
		return "", errs.Unsup("subtitle", "no subtitle blocks")
	}
	scale := head.Segment.Info.TimecodeScale
	// block is one subtitle cue: start time in ms and its text.
	type block struct {
		ts   int64
		dur  int64 // ms; -1 when the block has no BlockDuration
		text string
	}
	var blocks []block
	var total int64
	w := clusterWalker{r: r, segEnd: ix.segEnd, off: ix.firstCluster}
	for {
		bi, err := w.next()
		if err != nil {
			// io.EOF or a corrupt tail: keep the cues read so far.
			break
		}
		if bi.track != target || bi.payloadLen <= 0 {
			continue
		}
		frames, err := readFrames(r, bi)
		if err != nil {
			break
		}
		for _, fb := range frames {
			total += int64(len(fb))
			if total > maxCollectBytes {
				return "", errs.Unsup("subtitle", "subtitle track too large")
			}
			text := subtitlePayloadText(te.CodecID, fb)
			if text == "" {
				continue
			}
			dur := int64(-1)
			if bi.duration >= 0 {
				dur = timecodeTicksToMs(bi.duration, scaleOrDefault(scale))
			}
			blocks = append(blocks, block{ts: timecodeTicksToMs(bi.timecode, scaleOrDefault(scale)), dur: dur, text: text})
		}
	}
	if len(blocks) == 0 {
		return "", errs.Unsup("subtitle", "no subtitle blocks")
	}
	var cues []string
	for i, bl := range blocks {
		start := msToVTT(bl.ts)
		endMs := bl.ts + 2000
		if bl.dur > 0 {
			// BlockDuration is the cue's end, as ffmpeg uses it.
			endMs = bl.ts + bl.dur
		} else if i+1 < len(blocks) {
			endMs = blocks[i+1].ts
			if endMs <= bl.ts {
				endMs = bl.ts + 2000
			}
		}
		end := msToVTT(endMs)
		cues = append(cues, fmt.Sprintf("%s --> %s\n%s", start, end, bl.text))
	}
	if len(blocks) == 1 {
		raw := blocks[0].text
		if strings.Contains(raw, "Dialogue:") {
			return subtitle.ParseASS(strings.NewReader(raw))
		}
		if strings.Contains(raw, "-->") {
			return subtitle.ParseSRT(strings.NewReader(raw))
		}
	}
	return "WEBVTT\n\n" + strings.Join(cues, "\n\n") + "\n", nil
}

// subtitlePayloadText returns the displayable cue text of one Matroska
// subtitle frame, matching what ffmpeg's matroska demuxer passes on as the
// cue payload when it writes WebVTT:
//   - D_WEBVTT/SUBTITLES (WebM): the frame is "identifier\nsettings\npayload"
//     (WebM WebVTT mapping). The identifier and cue settings lines such as
//     "align:start position:0%" are not text; ffmpeg's WebVTT output drops
//     them, so they are dropped here too.
//   - S_TEXT/WEBVTT: settings and identifier live in BlockAdditions, so the
//     frame is already the payload.
//   - S_TEXT/ASS and S_TEXT/SSA: the frame is the Dialogue fields after the
//     times ("ReadOrder,Layer,Style,Name,MarginL,MarginR,MarginV,Effect,Text");
//     only Text is kept and override tags such as {\an7} are stripped, as
//     ffmpeg's ass-to-webvtt conversion does.
func subtitlePayloadText(codecID string, fb []byte) string {
	s := strings.ReplaceAll(string(fb), "\r\n", "\n")
	switch codecID {
	case "D_WEBVTT/SUBTITLES":
		parts := strings.SplitN(s, "\n", 3)
		if len(parts) < 3 {
			return ""
		}
		s = parts[2]
	case "S_TEXT/ASS", "S_TEXT/SSA":
		parts := strings.SplitN(s, ",", 9)
		if len(parts) == 9 {
			s = parts[8]
		}
		s = subtitle.StripASSTags(s)
	}
	return strings.TrimSpace(s)
}

// scaleOrDefault applies the 1 ms Matroska default to an unset TimecodeScale.
func scaleOrDefault(s uint64) uint64 {
	if s == 0 {
		return 1_000_000
	}
	return s
}

// msToVTT formats milliseconds as a WebVTT timestamp.
func msToVTT(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3_600_000
	ms %= 3_600_000
	m := ms / 60_000
	ms %= 60_000
	s := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}
