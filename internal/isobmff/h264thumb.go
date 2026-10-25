package isobmff

import (
	"image"
	"io"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

type h264ThumbCtx struct {
	track int
	avcc  *mp4.AVCDecoderConfiguration
	sample []byte
}

func loadH264Thumb(r io.ReaderAt, size int64, seekPercent float64) (h264ThumbCtx, bool) {
	info, err := mp4.Probe(ioat.New(r, size))
	if err != nil {
		return h264ThumbCtx{}, false
	}
	track := -1
	for i, tr := range info.Tracks {
		if tr.Codec == mp4.CodecAVC1 {
			track = i
			break
		}
	}
	if track < 0 {
		return h264ThumbCtx{}, false
	}
	avcc, ok := AVCDecoderConfig(r, size, track)
	if !ok {
		return h264ThumbCtx{}, false
	}
	sample, ok := h264SyncSampleFromInfo(r, size, track, seekPercent, info)
	if !ok {
		return h264ThumbCtx{}, false
	}
	return h264ThumbCtx{track: track, avcc: avcc, sample: sample}, true
}

// H264ThumbnailYCbCr decodes an IDR frame to YCbCr for area/box downscale before JPEG encode.
// Returns (nil, nil) when the file is not a decodable H.264 MP4 thumbnail path.
func H264ThumbnailYCbCr(r io.ReaderAt, size int64, seekPercent float64, fast bool) (*h264.ThumbYCbCr, error) {
	ctx, ok := loadH264Thumb(r, size, seekPercent)
	if !ok {
		return nil, nil
	}
	fr, err := h264.DecodeAVCCFrame(ctx.avcc, ctx.sample, fast)
	if err != nil {
		return nil, err
	}
	if fr == nil {
		return nil, nil
	}
	return h264.NewThumbYCbCr(fr), nil
}

// H264ThumbnailRGBA seeks to seekPercent and decodes an H.264 IDR frame to RGBA.
// Returns (nil, nil) when the file is not a decodable H.264 MP4 thumbnail path.
func H264ThumbnailRGBA(r io.ReaderAt, size int64, seekPercent float64, fast bool) (*image.RGBA, error) {
	ctx, ok := loadH264Thumb(r, size, seekPercent)
	if !ok {
		return nil, nil
	}
	img, err := h264.DecodeAVCC(ctx.avcc, ctx.sample, fast)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}
	return img, nil
}
