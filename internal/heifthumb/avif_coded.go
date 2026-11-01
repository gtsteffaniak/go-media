package heifthumb

import (
	"io"

	"github.com/gtsteffaniak/go-media/internal/av1decode"
	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

const maxAVIFBytes = 48 << 20

// CodedAVIFYCbCr decodes the primary AV1-coded AVIF item (still or grid).
func CodedAVIFYCbCr(r io.ReaderAt, size int64) (*h264.ThumbYCbCr, error) {
	if size <= 0 || size > maxAVIFBytes {
		return nil, errs.Unsup("thumbnail", "avif: file size out of range for coded decode")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(io.NewSectionReader(r, 0, size), data); err != nil {
		return nil, err
	}
	ycc, err := av1decode.DecodeAVIFFile(data)
	if err != nil {
		return nil, errs.Unsup("thumbnail", "avif coded image: "+err.Error())
	}
	if ycc == nil {
		return nil, errs.UnsupProfile("avif", "decoder returned no picture")
	}
	return ycc, nil
}
