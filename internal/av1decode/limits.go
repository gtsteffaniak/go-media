package av1decode

import (
	"fmt"

	"github.com/gtsteffaniak/go-media/internal/errs"
)

// DefaultFrameLongSide and DefaultFrameShortSide are the library's default
// DCI 4K per-side caps when the caller leaves MaxAV1FrameLongSide and
// MaxAV1FrameShortSide unset in media.ThumbnailOptions.
const (
	DefaultFrameLongSide  = 4096
	DefaultFrameShortSide = 2160
)

// FrameLimits caps coded AV1 video keyframe dimensions for one decode. When
// both LongSide and ShortSide are zero, resolved() uses the package variables
// MaxFrameLongSide and MaxFrameShortSide (4096×2160 by default; both zero
// disables the cap). When the caller sets either side, a zero on one side is
// filled with DefaultFrameLongSide or DefaultFrameShortSide.
type FrameLimits struct {
	LongSide  int
	ShortSide int
}

func (l FrameLimits) resolved() (long, short int) {
	if l.LongSide == 0 && l.ShortSide == 0 {
		return MaxFrameLongSide, MaxFrameShortSide
	}
	long, short = l.LongSide, l.ShortSide
	if long == 0 {
		long = DefaultFrameLongSide
	}
	if short == 0 {
		short = DefaultFrameShortSide
	}
	return long, short
}

func (l FrameLimits) frameSizeLimit() int {
	long, short := l.resolved()
	if long <= 0 || short <= 0 {
		return 0
	}
	return long * short
}

func (l FrameLimits) checkFrameSize(w, h int) error {
	long, short := l.resolved()
	if long <= 0 || short <= 0 {
		return nil
	}
	lo, sh := max(w, h), min(w, h)
	if lo > long || sh > short {
		return l.frameTooLarge(fmt.Sprintf("frame %dx%d", w, h))
	}
	return nil
}

func (l FrameLimits) frameTooLarge(what string) error {
	long, short := l.resolved()
	return errs.UnsupProfile("av1 keyframe", fmt.Sprintf("%s exceeds the %dx%d thumbnail limit", what, long, short))
}

// avifDecodeLimit returns the FrameSizeLimit handed to gav1d for AVIF: the
// caller's maxSourcePixels when positive, clamped to MaxAVIFPixels; otherwise
// MaxAVIFPixels alone.
func avifDecodeLimit(maxSourcePixels int64) int64 {
	if maxSourcePixels <= 0 {
		return MaxAVIFPixels
	}
	if maxSourcePixels > MaxAVIFPixels {
		return MaxAVIFPixels
	}
	return maxSourcePixels
}
