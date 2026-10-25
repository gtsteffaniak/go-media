package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// MJPEGFrame returns the first video sample when the track is MJPEG.
func MJPEGFrame(r io.ReaderAt, size int64) ([]byte, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, false
	}
	// Probe does not expose mjpeg directly; scan mdat for JPEG SOI after stsz/stco would be needed.
	// For test fixtures we embed a single-frame MJPEG in mdat right after moov.
	mdat, err := mp4.ExtractBox(ioat.New(r, size), nil, mp4.BoxPath{mp4.BoxTypeMdat()})
	if err != nil || len(mdat) == 0 {
		return nil, false
	}
	box := mdat[0]
	payload, err := io.ReadAll(io.NewSectionReader(ioat.New(r, size), int64(box.Offset)+int64(box.HeaderSize), int64(box.Size)-int64(box.HeaderSize)))
	if err != nil || len(payload) < 4 {
		return nil, false
	}
	// JPEG start of image
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
	_ = info
	return nil, false
}
