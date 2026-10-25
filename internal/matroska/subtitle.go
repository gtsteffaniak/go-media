package matroska

import (
	"fmt"
	"io"
	"strings"

	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/ioat"
	"github.com/gtsteffaniak/go-media/internal/subtitle"
)

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
	reader := io.NewSectionReader(ioat.New(r, size), 0, size)
	readers, err := mkvcore.NewSimpleBlockReader(reader)
	if err != nil {
		return "", err
	}
	var target uint64 = te.TrackNumber
	if target == 0 {
		target = uint64(streamIndex + 1)
	}
	type block struct {
		ts   int64
		text string
	}
	var blocks []block
	for _, br := range readers {
		entry := br.TrackEntry()
		if entry.TrackNumber != target {
			br.Close()
			continue
		}
		for {
			b, _, ts, err := br.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			text := strings.TrimSpace(string(b))
			if text == "" {
				continue
			}
			blocks = append(blocks, block{ts: ts, text: text})
		}
		br.Close()
	}
	if len(blocks) == 0 {
		return "", errs.Unsup("subtitle", "no subtitle blocks")
	}
	var cues []string
	for i, bl := range blocks {
		start := msToVTT(bl.ts)
		endMs := bl.ts + 2000
		if i+1 < len(blocks) {
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
