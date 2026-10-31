package media

import (
	"io"

	ts "github.com/gtsteffaniak/go-media/internal/mpegts"
)

// MPEGTSVideoElementary demuxes the primary video elementary stream from MPEG-TS/M2TS.
// The returned codec name is one of h264, mpeg2video, mpeg1video, mpeg4, hevc.
func MPEGTSVideoElementary(r io.ReaderAt, size int64) (es []byte, codec string, err error) {
	meta, err := ts.Probe(r, size)
	if err != nil {
		return nil, "", mapErr(err)
	}
	es, err = ts.VideoES(r, size, meta)
	if err != nil {
		return nil, "", mapErr(err)
	}
	if meta.VideoPID == 0 {
		return nil, "", Unsupported("mpegts", "no video PID")
	}
	for _, s := range meta.Streams {
		if s.PID == meta.VideoPID {
			return es, s.Codec, nil
		}
	}
	return es, "mpeg2video", nil
}
