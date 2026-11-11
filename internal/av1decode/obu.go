package av1decode

// maxShownFramesPerSample bounds how many pictures one sample may show while
// decoding. OBUs are submitted one at a time and Receive is drained between
// them so at most one decoded picture is queued; this cap still rejects samples
// that try to decode an excessive number of in-limit frames in one call.
const maxShownFramesPerSample = 16

// nextOBU splits the leading low-overhead OBU (size field required) from src.
func nextOBU(src []byte) (obu, rest []byte, ok bool) {
	if len(src) == 0 {
		return nil, nil, false
	}
	hdr := 1
	if src[0]&0x04 != 0 {
		hdr = 2
	}
	if src[0]&0x02 == 0 || len(src) <= hdr {
		return nil, nil, false
	}
	size, n := 0, 0
	for i := 0; i < 8; i++ {
		if hdr+i >= len(src) {
			return nil, nil, false
		}
		b := src[hdr+i]
		size |= int(b&0x7f) << (7 * i)
		n++
		if b&0x80 == 0 {
			break
		}
	}
	end := hdr + n + size
	if end > len(src) {
		return nil, nil, false
	}
	return src[:end], src[end:], true
}

func decodePixelBudget() int64 {
	limit := int64(frameSizeLimit())
	if limit <= 0 {
		limit = int64(4096 * 2160)
	}
	return limit * int64(maxShownFramesPerSample)
}
