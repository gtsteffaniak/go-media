package av1decode

import "math"

// maxShownFramesPerSample bounds how many pictures one sample may show while
// decoding. OBUs are submitted one at a time and Receive is drained between
// them so at most one decoded picture is queued; this cap still rejects samples
// that try to decode an excessive number of in-limit frames in one call.
const maxShownFramesPerSample = 16

// maxFrameOBUsPerSample bounds frame, frame-header, and redundant frame-header
// OBUs per sample. Frames with show_frame=0 are still decoded inside gav1d but
// never surface from Receive, so shown-frame limits alone are insufficient.
const maxFrameOBUsPerSample = 16

const (
	obuTypeFrameHdr          = 3
	obuTypeFrame             = 6
	obuTypeRedundantFrameHdr = 7
)

func obuType(obu []byte) int {
	if len(obu) == 0 {
		return -1
	}
	return int((obu[0] >> 3) & 0x0f)
}

func obuTriggersFrameDecode(typ int) bool {
	switch typ {
	case obuTypeFrameHdr, obuTypeFrame, obuTypeRedundantFrameHdr:
		return true
	default:
		return false
	}
}

// nextOBU splits the leading low-overhead OBU from src. OBUs with
// obu_has_size_field set carry a LEB128 payload length; when the flag is clear,
// the OBU is assumed to fill the remainder of the buffer. Per AV1-ISOBMFF §2.4
// only the last OBU in a sample may omit the size field.
func nextOBU(src []byte) (obu, rest []byte, ok bool) {
	if len(src) == 0 {
		return nil, nil, false
	}
	hdr := 1
	if src[0]&0x04 != 0 {
		hdr = 2
	}
	if len(src) <= hdr {
		return nil, nil, false
	}
	if src[0]&0x02 == 0 {
		return src, nil, true
	}
	var size uint64
	n := 0
	for i := 0; i < 8; i++ {
		if hdr+i >= len(src) {
			return nil, nil, false
		}
		b := src[hdr+i]
		size |= uint64(b&0x7f) << (7 * i)
		n++
		if b&0x80 == 0 {
			break
		}
	}
	end64 := uint64(hdr+n) + size
	if end64 > uint64(len(src)) {
		return nil, nil, false
	}
	end := int(end64)
	if uint64(end) != end64 {
		return nil, nil, false
	}
	return src[:end], src[end:], true
}

// framePixels returns w*h as an int64 pixel count, reporting false when
// either dimension is negative or the product would overflow. Decoded
// picture dimensions come from the bitstream, so they are not trusted
// to fit in an int64 product.
func framePixels(w, h int) (int64, bool) {
	if w < 0 || h < 0 {
		return 0, false
	}
	if w == 0 || h == 0 {
		return 0, true
	}
	if int64(w) > math.MaxInt64/int64(h) {
		return 0, false
	}
	return int64(w) * int64(h), true
}

// decodePixelBudget returns the aggregate pixel budget for one sample:
// the per-frame limit × maxShownFramesPerSample, saturating at
// math.MaxInt64 instead of wrapping. With a saturated per-frame limit
// (see frameSizeLimit) the naive product wraps to zero or negative and
// every picture, including small in-limit ones, fails the aggregate
// check in decodeSampleLastPicture.
func decodePixelBudget(lim FrameLimits) int64 {
	limit := int64(lim.frameSizeLimit())
	if limit <= 0 {
		limit = int64(DefaultFrameLongSide * DefaultFrameShortSide)
	}
	if limit > math.MaxInt64/int64(maxShownFramesPerSample) {
		return math.MaxInt64
	}
	return limit * int64(maxShownFramesPerSample)
}
