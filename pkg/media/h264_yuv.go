package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

// H264KeyframeYUV420 decodes the sync sample at seekPercent to planar I420
// bytes (for metrics). Samples are returned as coded, with no range or matrix
// conversion. Open-GOP keyframes are accepted as in Thumbnail.
func H264KeyframeYUV420(r io.ReaderAt, size int64, seekPercent float64, fast bool) (out []byte, w, h int, err error) {
	defer func() {
		if p := recover(); p != nil {
			out = nil
			w, h = 0, 0
			err = panicErr(p)
		}
	}()
	track, ok := isobmff.FirstAVCTrackIndex(r, size)
	if !ok {
		return nil, 0, 0, Unsupported("h264 yuv", "no avc track")
	}
	avcc, ok := isobmff.AVCDecoderConfig(r, size, track)
	if !ok {
		return nil, 0, 0, Unsupported("h264 yuv", "no avcC")
	}
	sample, ok := isobmff.H264SyncSample(r, size, track, seekPercent)
	if !ok {
		return nil, 0, 0, Unsupported("h264 yuv", "no sync sample")
	}
	planes, err := h264.DecodeAVCCYUV420(avcc, sample, fast)
	if err != nil {
		return nil, 0, 0, mapErr(err)
	}
	out = planes.WriteI420(nil)
	return out, planes.Width, planes.Height, nil
}

// H264KeyframeTimeSec returns the decode time (DTS, in seconds) of the selected
// sync sample. It ignores ctts offsets, so for streams with B-frames it is not
// the presentation time. Use it as the ffmpeg -ss value for the sample's decode
// position, and check the compare gate on any new B-frame sample.
func H264KeyframeTimeSec(r io.ReaderAt, size int64, seekPercent float64) (float64, error) {
	track, ok := isobmff.FirstAVCTrackIndex(r, size)
	if !ok {
		return 0, Unsupported("h264 sync time", "no avc track")
	}
	sec, ok := isobmff.H264SyncTimeSec(r, size, track, seekPercent)
	if !ok {
		return 0, Unsupported("h264 sync time", "no sync sample")
	}
	return sec, nil
}
