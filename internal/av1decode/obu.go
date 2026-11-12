package av1decode

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

func decodePixelBudget(lim FrameLimits) int64 {
	limit := int64(lim.frameSizeLimit())
	if limit <= 0 {
		limit = int64(DefaultFrameLongSide * DefaultFrameShortSide)
	}
	return limit * int64(maxShownFramesPerSample)
}
