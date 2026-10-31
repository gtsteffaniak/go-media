package mpeg2video

import (
	"bytes"
	"errors"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// DecodeIFrameIndex decodes the nth I-picture in es (0-based). It is used for
// MPEG-TS/PS seeking when the elementary stream holds many pictures.
func DecodeIFrameIndex(es []byte, index int) (*Picture, error) {
	if len(es) == 0 {
		return nil, corruptMPEG2("empty elementary stream")
	}
	if index < 0 {
		index = 0
	}
	vs := NewVideoSequence(bytes.NewReader(es))
	n := 0
	var last *Picture
	for {
		img, err := vs.step()
		if errors.Is(err, EOS) {
			if last != nil {
				return last, nil
			}
			return nil, errs.UnsupProfile("mpeg2 keyframe", "no I-picture at seek index")
		}
		if err != nil {
			if last != nil {
				return last, nil
			}
			return nil, mapDecodeErr(err)
		}
		if vs.PictureHeader == nil || vs.PictureHeader.picture_coding_type != IFrame {
			continue
		}
		w, h := vs.Size()
		if err := checkDimensions(w, h); err != nil {
			return nil, err
		}
		pic := pictureFromYCbCr(img, w, h)
		last = pic
		if n == index {
			return pic, nil
		}
		n++
	}
}
