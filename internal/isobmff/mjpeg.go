package isobmff

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// MJPEGFrame returns a JPEG sample from an MJPEG-in-MP4 track at seekPercent of duration.
func MJPEGFrame(r io.ReaderAt, size int64, seekPercent float64) ([]byte, bool) {
	idx, err := openMovieIndex(r, size)
	if err != nil {
		return nil, false
	}
	videoTrack := -1
	for i := range idx.tracks {
		switch string(idx.tracks[i].entry[:]) {
		case "avc1", "encv":
			return nil, false
		case "mp4v":
			if _, ok := visualMJPEGSampleEntry(r, size, i); ok {
				videoTrack = i
			}
		}
		if videoTrack >= 0 {
			break
		}
	}
	if videoTrack < 0 {
		return nil, false
	}
	tr := &idx.tracks[videoTrack]
	if !tr.st.usable() {
		return nil, false
	}
	sample := tr.nearestSample(seekPercent)
	off, n, err := tr.st.locate(sample)
	if err != nil || n <= 0 {
		return nil, false
	}
	payload, err := ioat.ReadAll(r, size, off, n)
	if err != nil || len(payload) < 4 {
		return nil, false
	}
	if b, ok := extractJPEG(payload); ok {
		return b, true
	}
	return nil, false
}

// nearestSample returns the sample whose decode time is closest to seekPercent
// of the track duration (every MJPEG sample is a keyframe).
func (tr *trackIndex) nearestSample(seekPercent float64) int {
	target := uint64(float64(tr.durationTicks()) * normalizeSeekPercent(seekPercent) / 100.0)
	i := tr.st.sampleAtTime(target)
	if uint32(i+1) < tr.st.sampleCount {
		if next := tr.st.sampleTime(i + 1); next-target < target-tr.st.sampleTime(i) {
			return i + 1
		}
	}
	return i
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
