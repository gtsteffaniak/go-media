package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

const maxMJPEGSniff = 8 * 1024 * 1024

// MJPEGFrame returns the first JPEG in mdat when the file looks like a single-frame MJPEG MP4.
func MJPEGFrame(r io.ReaderAt, size int64) ([]byte, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return nil, false
	}
	// Skip typical H.264/AAC MP4 files without reading mdat.
	for _, tr := range info.Tracks {
		if tr.Codec == mp4.CodecAVC1 {
			return nil, false
		}
	}
	mdat, err := mp4.ExtractBox(ioat.New(r, size), nil, mp4.BoxPath{mp4.BoxTypeMdat()})
	if err != nil || len(mdat) == 0 {
		return nil, false
	}
	box := mdat[0]
	payloadLen := int64(box.Size) - int64(box.HeaderSize)
	if payloadLen <= 0 {
		return nil, false
	}
	readLen := payloadLen
	if readLen > maxMJPEGSniff {
		readLen = maxMJPEGSniff
	}
	payload, err := ioat.ReadAll(r, size, int64(box.Offset)+int64(box.HeaderSize), readLen)
	if err != nil || len(payload) < 4 {
		return nil, false
	}
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
