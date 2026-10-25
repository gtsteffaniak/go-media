package nal

// SplitAnnexB splits an Annex-B bytestream into "raw" NAL units (EBSP, with
// the header byte but without start codes). A start code is 00 00 01 (3 bytes)
// or 00 00 00 01 (4 bytes). The extra leading 0 of the four-byte code is
// assigned to the separator, not to the NAL body.
func SplitAnnexB(data []byte) [][]byte {
	starts := findStartCodes(data)
	if len(starts) == 0 {
		return nil
	}
	units := make([][]byte, 0, len(starts))
	for i := range starts {
		begin := starts[i].end // right after the start code
		stop := len(data)
		if i+1 < len(starts) {
			stop = starts[i+1].start
		}
		if stop > begin {
			units = append(units, data[begin:stop])
		}
	}
	return units
}

// ParseAnnexB — a convenience wrapper: Annex-B bitstream → parsed NAL units.
func ParseAnnexB(data []byte) []Unit {
	raw := SplitAnnexB(data)
	out := make([]Unit, 0, len(raw))
	for _, r := range raw {
		if u, ok := Parse(r); ok {
			out = append(out, u)
		}
	}
	return out
}

type startCode struct{ start, end int } // [start:end) — the bytes of the start code itself

// findStartCodes finds all 00 00 01 start codes, absorbing the optional
// leading 0 (for 00 00 00 01).
func findStartCodes(d []byte) []startCode {
	var out []startCode
	i := 0
	for i+3 <= len(d) {
		if d[i] == 0 && d[i+1] == 0 && d[i+2] == 1 {
			start := i
			if start > 0 && d[start-1] == 0 {
				start-- // 00 00 00 01 — take the extra leading zero into the separator
			}
			out = append(out, startCode{start: start, end: i + 3})
			i += 3
			continue
		}
		i++
	}
	return out
}
