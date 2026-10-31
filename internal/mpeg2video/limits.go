package mpeg2video

import "github.com/gtsteffaniak/go-media/internal/errs"

const (
	// MaxWidth and MaxHeight bound allocations for thumbnail decode.
	MaxWidth  = 4096
	MaxHeight = 4096
	maxPixels = MaxWidth * MaxHeight
)

func checkDimensions(w, h int) error {
	if w > MaxWidth || h > MaxHeight {
		return errs.UnsupProfile("mpeg2 keyframe", "picture dimensions exceed thumbnail limits")
	}
	if w*h > maxPixels {
		return errs.UnsupProfile("mpeg2 keyframe", "picture pixel count exceeds thumbnail limits")
	}
	return nil
}
