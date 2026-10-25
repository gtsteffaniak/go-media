package matroska

import (
	"bytes"
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
	var buf bytes.Buffer
	for _, br := range readers {
		entry := br.TrackEntry()
		if entry.TrackNumber != target {
			br.Close()
			continue
		}
		for {
			b, _, _, err := br.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			buf.Write(b)
			buf.WriteByte('\n')
		}
		br.Close()
	}
	raw := buf.String()
	if strings.Contains(raw, "Dialogue:") {
		return subtitle.ParseASS(strings.NewReader(raw))
	}
	if strings.Contains(raw, "-->") {
		return subtitle.ParseSRT(strings.NewReader(raw))
	}
	return subtitle.ParseVTT(strings.NewReader("WEBVTT\n\n" + raw))
}
