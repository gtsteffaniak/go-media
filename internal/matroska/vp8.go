package matroska

import (
	"io"

	"github.com/at-wat/ebml-go/mkvcore"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// VP8Keyframe returns a VP8 keyframe payload near seekPercent of the file timeline.
func VP8Keyframe(r io.ReaderAt, size int64, seekPercent float64) ([]byte, bool) {
	reader := io.NewSectionReader(ioat.New(r, size), 0, size)
	readers, err := mkvcore.NewSimpleBlockReader(reader)
	if err != nil {
		return nil, false
	}
	var keyframes [][]byte
	for _, br := range readers {
		entry := br.TrackEntry()
		if entry.CodecID != "V_VP8" {
			br.Close()
			continue
		}
		for {
			b, key, _, err := br.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			if key {
				keyframes = append(keyframes, append([]byte(nil), b...))
			}
		}
		br.Close()
	}
	if len(keyframes) == 0 {
		return nil, false
	}
	idx := int(float64(len(keyframes)-1) * seekPercent / 100.0)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(keyframes) {
		idx = len(keyframes) - 1
	}
	return keyframes[idx], true
}
