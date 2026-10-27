package isobmff

import (
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

// H264MP4Thumb caches one mp4.Probe and avcC per file for repeated thumbnails (same ReaderAt).
type H264MP4Thumb struct {
	R     io.ReaderAt
	Size  int64
	Track int
	Probe *mp4.ProbeInfo
	AVCC  *mp4.AVCDecoderConfiguration
}

// OpenH264MP4Thumb probes the file once and reads avcC for the first H.264 track.
func OpenH264MP4Thumb(r io.ReaderAt, size int64) (H264MP4Thumb, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return H264MP4Thumb{}, false
	}
	track := -1
	for i, tr := range info.Tracks {
		if tr.Codec == mp4.CodecAVC1 {
			track = i
			break
		}
	}
	if track < 0 {
		return H264MP4Thumb{}, false
	}
	avcc, ok := AVCDecoderConfig(r, size, track)
	if !ok {
		return H264MP4Thumb{}, false
	}
	return H264MP4Thumb{R: r, Size: size, Track: track, Probe: info, AVCC: avcc}, true
}

// ThumbnailYCbCr decodes an IDR at seekPercent using cached probe state.
func (t H264MP4Thumb) ThumbnailYCbCr(seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	sample, ok := h264SyncSampleFromInfo(t.R, t.Size, t.Track, seekPercent, t.Probe)
	if !ok {
		return nil, nil
	}
	fr, err := h264.DecodeAVCCFrame(t.AVCC, sample, fast)
	if err != nil {
		return nil, err
	}
	if fr == nil {
		return nil, nil
	}
	return h264.NewThumbYCbCr(fr), nil
}
