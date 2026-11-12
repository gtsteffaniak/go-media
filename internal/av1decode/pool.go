package av1decode

import (
	"github.com/gtsteffaniak/go-media/internal/h264"
)

// DecodeSampleCached decodes one keyframe. It keeps its historical name, but
// no decoder is cached any more: gav1d allocates frame memory per frame
// rather than sizing arenas up front, so building a decoder is cheap, and a
// fresh decoder per call means nothing is held between thumbnails. Peak
// memory is bounded by MaxFrameLongSide × MaxFrameShortSide per concurrent
// call; held memory after the call is zero. Concurrent callers never share a
// decoder.
func DecodeSampleCached(av1CExtradata, sample []byte, lim FrameLimits) (*h264.ThumbYCbCr, error) {
	d := NewDecoderWithLimits(lim)
	defer d.Close()
	return d.DecodeSample(av1CExtradata, sample)
}
