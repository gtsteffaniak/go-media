package cavlc

type coeffTokenEntry struct {
	tc, t1 int
	codeLen int
	ok      bool
}

var coeffTokenLUT [4][1 << 16]coeffTokenEntry

func init() {
	for tableIdx := range 4 {
		maxLen := coeffTokenMaxLen[tableIdx]
		for tc := 0; tc <= 16; tc++ {
			maxT1 := min(tc, 3)
			for t1 := 0; t1 <= maxT1; t1++ {
				idx := 4*tc + t1
				codeLen := int(coeffTokenLen[tableIdx][idx])
				if codeLen == 0 {
					continue
				}
				codeBits := uint32(coeffTokenBits[tableIdx][idx])
				limit := 1 << maxLen
				for peek := 0; peek < limit; peek++ {
					shifted := uint32(peek) >> uint(maxLen-codeLen)
					if shifted == codeBits {
						coeffTokenLUT[tableIdx][peek] = coeffTokenEntry{tc: tc, t1: t1, codeLen: codeLen, ok: true}
					}
				}
			}
		}
	}
}

func decodeCoeffTokenFast(br *BitReader, tableIdx int) (totalCoeff, trailingOnes int, ok bool) {
	maxLen := coeffTokenMaxLen[tableIdx]
	peeked, err := br.PeekBits(maxLen)
	if err != nil {
		return 0, 0, false
	}
	e := coeffTokenLUT[tableIdx][peeked]
	if !e.ok {
		return 0, 0, false
	}
	br.SkipBits(e.codeLen)
	return e.tc, e.t1, true
}
