package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// MJPEGFrame returns a JPEG sample from an MJPEG-in-MP4 track at seekPercent of duration.
func MJPEGFrame(r io.ReaderAt, size int64, seekPercent float64) ([]byte, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, false
	}
	videoTrack := -1
	for i, tr := range info.Tracks {
		if tr.Codec == mp4.CodecAVC1 {
			return nil, false
		}
		if tr.Codec == mp4.CodecUnknown {
			if _, ok := visualMJPEGSampleEntry(r, size, i); ok {
				videoTrack = i
				break
			}
		}
	}
	if videoTrack < 0 {
		return nil, false
	}
	refs, err := SampleRefs(r, size, videoTrack)
	if err != nil || len(refs) == 0 {
		return nil, false
	}
	ref, ok := pickSampleByTimeFromInfo(info, videoTrack, refs, seekPercent)
	if !ok {
		ref = refs[0]
	}
	if ref.Size <= 0 {
		return nil, false
	}
	payload, err := ioat.ReadAll(r, size, ref.Offset, ref.Size)
	if err != nil || len(payload) < 4 {
		return nil, false
	}
	if b, ok := extractJPEG(payload); ok {
		return b, true
	}
	return nil, false
}

func extractJPEG(payload []byte) ([]byte, bool) {
	for i := 0; i+2 < len(payload); i++ {
		if payload[i] == 0xFF && payload[i+1] == 0xD8 {
			end := i + 2
			for ; end+1 < len(payload); end++ {
				if payload[end] == 0xFF && payload[end+1] == 0xD9 {
					end += 2
					return payload[i:end], true
				}
			}
			return payload[i:], true
		}
	}
	return nil, false
}
