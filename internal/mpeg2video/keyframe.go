package mpeg2video

import (
	"bytes"
	"errors"
	"fmt"
	"image"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

func corruptMPEG2(reason string) error {
	return fmt.Errorf("media: corrupt mpeg2: %s: %w", reason, errs.Corrupt)
}

// Picture is a decoded 4:2:0 I-picture. Width and Height are the display size.
type Picture struct {
	Y, Cb, Cr        []byte
	YStride, CStride int
	Width, Height    int
}

// DecodeKeyframe decodes the first I-picture in es, an elementary stream with
// MPEG-2 start codes (as in MP4 samples, PES payloads, or TS PES). It returns
// errs.UnsupportedProfile when the first decodable picture is not intra-coded,
// or when a coding extension the thumbnail path does not implement is present.
func DecodeKeyframe(es []byte) (*Picture, error) {
	if len(es) == 0 {
		return nil, corruptMPEG2("empty elementary stream")
	}
	vs := NewVideoSequence(bytes.NewReader(es))
	for {
		img, err := vs.step()
		if errors.Is(err, EOS) {
			return nil, errs.UnsupProfile("mpeg2 keyframe", "no I-picture in bitstream")
		}
		if err != nil {
			if errors.Is(err, ErrUnsupportedVideoStream_ISO_IEC_11172_2) {
				return nil, errs.UnsupProfile("mpeg2 keyframe", "MPEG-1 video (ISO/IEC 11172-2) not supported")
			}
			return nil, mapDecodeErr(err)
		}
		if vs.PictureHeader == nil {
			continue
		}
		if vs.PictureHeader.picture_coding_type != IFrame {
			return nil, errs.UnsupProfile("mpeg2 keyframe",
				fmt.Sprintf("sync sample starts with %s-picture, not I (only intra pictures are decoded)",
					vs.PictureHeader.picture_coding_type))
		}
		w, h := vs.Size()
		if w <= 0 || h <= 0 {
			return nil, corruptMPEG2("invalid picture size")
		}
		if err := checkDimensions(w, h); err != nil {
			return nil, err
		}
		return pictureFromYCbCr(img, w, h), nil
	}
}

func pictureFromYCbCr(img *image.YCbCr, w, h int) *Picture {
	if img == nil {
		return nil
	}
	return &Picture{
		Y: img.Y, Cb: img.Cb, Cr: img.Cr,
		YStride: img.YStride, CStride: img.CStride,
		Width: w, Height: h,
	}
}

func mapDecodeErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	switch {
	case errors.Is(err, ErrInvalidReservedBits), errors.Is(err, ErrMissingMarkerBit),
		errors.Is(err, ErrUnexpectedStartCode):
		return corruptMPEG2(msg)
	default:
		return errs.Unsup("mpeg2 keyframe", msg)
	}
}
