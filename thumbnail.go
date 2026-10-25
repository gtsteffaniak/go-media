package media

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/thumb"
)

// Thumbnail returns a scaled JPEG or PNG preview.
func Thumbnail(r io.ReaderAt, size int64, opt ThumbnailOptions) (ThumbnailResult, error) {
	res, err := thumb.Generate(r, size, thumb.Options{
		MaxWidth:       opt.MaxWidth,
		MaxHeight:      opt.MaxHeight,
		SeekPercent:    opt.SeekPercent,
		Format:         opt.Format,
		JPEGQuality:    opt.JPEGQuality,
		Fast:           opt.Fast,
		PreferEmbedded: opt.PreferEmbedded,
	})
	if err != nil {
		return ThumbnailResult{}, mapErr(err)
	}
	return ThumbnailResult{
		MIME:   res.MIME,
		Data:   res.Data,
		Source: Source(res.Source),
		Width:  res.Width,
		Height: res.Height,
	}, nil
}

// DecodeH264Keyframe exposes the intra-only H.264 decoder entry point (stub).
func DecodeH264Keyframe(nal []byte, width, height int) ([]byte, int, int, error) {
	data, w, h, err := h264.DecodeKeyframe(nal, width, height)
	if err != nil {
		return nil, 0, 0, UnsupportedProfile("h264 keyframe", "intra decoder not implemented")
	}
	return data, w, h, nil
}
