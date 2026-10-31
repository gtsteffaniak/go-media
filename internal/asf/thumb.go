package asf

import (
	"image"

	"github.com/gtsteffaniak/go-media/internal/errs"
	"github.com/gtsteffaniak/go-media/internal/h264"
	"github.com/gtsteffaniak/go-media/internal/h264/yuv"
	"github.com/gtsteffaniak/go-media/internal/vc1"
)

// ThumbSource indexes an ASF file for WMV3 thumbnails.
type ThumbSource struct {
	meta *fileMeta
	rd   reader
	size int64
}

// OpenThumbSourceErr parses ASF headers for thumbnail decode.
func OpenThumbSourceErr(r interface {
	ReadAt([]byte, int64) (int, error)
}, size int64) (ThumbSource, error) {
	meta, err := openMeta(r, size)
	if err != nil {
		return ThumbSource{}, err
	}
	return ThumbSource{meta: meta, rd: newReader(r, size), size: size}, nil
}

// WMV3ThumbnailYCbCr decodes a WMV3 I-picture near seekPercent.
func (s ThumbSource) WMV3ThumbnailYCbCr(seekPercent float64) (*h264.ThumbYCbCr, error) {
	if s.meta.video == nil || s.meta.video.codec != "wmv3" {
		return nil, nil
	}
	extra, sample, err := videoKeyframe(s.rd.r, s.size, seekPercent)
	if err != nil {
		return nil, err
	}
	pic, err := vc1.DecodeKeyframe(extra, sample, s.meta.video.width, s.meta.video.height)
	if err != nil {
		return nil, err
	}
	return vc1PictureToThumb(pic), nil
}

// UnsupportedReason reports when no decoder matched.
func (s ThumbSource) UnsupportedReason() error {
	if s.meta.video == nil {
		return errs.Unsup("thumbnail", "asf: no video stream")
	}
	return errs.Unsup("thumbnail", "asf: video codec "+s.meta.video.codec+" not supported")
}

func vc1PictureToThumb(p *vc1.Picture) *h264.ThumbYCbCr {
	if p == nil {
		return nil
	}
	ycc := &image.YCbCr{
		Y: p.Y, Cb: p.Cb, Cr: p.Cr,
		YStride: p.YStride, CStride: p.CStride,
		SubsampleRatio: image.YCbCrSubsampleRatio420,
		Rect:           image.Rect(0, 0, p.Width, p.Height),
	}
	rng := yuv.LimitedRange
	if p.FullRange {
		rng = yuv.FullRange
	}
	return &h264.ThumbYCbCr{YCbCr: ycc, CS: yuv.BT601, RNG: rng}
}
