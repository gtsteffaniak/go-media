package cavlc

// Table-driven VLC decode for total_zeros and run_before (ffmpeg h264_cavlc.c style).

type vlcEntry struct {
	value   int
	codeLen int
	ok      bool
}

const totalZerosMaxPeek = 9
const runBeforeMaxPeek = 11

var totalZeros4x4LUT [15][1 << totalZerosMaxPeek]vlcEntry
var chromaDCTotalZerosLUT [3][1 << 3]vlcEntry
var runBeforeLUT [7][1 << runBeforeMaxPeek]vlcEntry

func initVLCRowLUT(lut []vlcEntry, maxPeek int, maxValue int, lenRow, bitsRow []uint8) {
	for v := 0; v <= maxValue; v++ {
		codeLen := int(lenRow[v])
		if codeLen == 0 {
			continue
		}
		codeBits := uint32(bitsRow[v])
		limit := 1 << maxPeek
		for peek := 0; peek < limit; peek++ {
			shifted := uint32(peek) >> uint(maxPeek-codeLen)
			if shifted == codeBits {
				lut[peek] = vlcEntry{value: v, codeLen: codeLen, ok: true}
			}
		}
	}
}

func init() {
	for row := 0; row < 15; row++ {
		maxZeros := 15 - row
		lenSlice := make([]uint8, 16)
		bitsSlice := make([]uint8, 16)
		for tz := 0; tz < 16; tz++ {
			lenSlice[tz] = totalZerosLen[row][tz]
			bitsSlice[tz] = totalZerosBits[row][tz]
		}
		initVLCRowLUT(totalZeros4x4LUT[row][:], totalZerosMaxPeek, maxZeros, lenSlice, bitsSlice)
	}

	for row := 0; row < 3; row++ {
		maxZeros := 4 - (row + 1)
		lenSlice := make([]uint8, 4)
		bitsSlice := make([]uint8, 4)
		for tz := 0; tz < 4; tz++ {
			lenSlice[tz] = chromaDCTotalZerosLen[row][tz]
			bitsSlice[tz] = chromaDCTotalZerosBits[row][tz]
		}
		initVLCRowLUT(chromaDCTotalZerosLUT[row][:], 3, maxZeros, lenSlice, bitsSlice)
	}

	for row := 0; row < 7; row++ {
		maxRB := row + 1
		if row == 6 {
			maxRB = 14
		}
		lenSlice := make([]uint8, 16)
		bitsSlice := make([]uint8, 16)
		for rb := 0; rb < 16; rb++ {
			lenSlice[rb] = runBeforeLen[row][rb]
			bitsSlice[rb] = runBeforeBits[row][rb]
		}
		initVLCRowLUT(runBeforeLUT[row][:], runBeforeMaxPeek, maxRB, lenSlice, bitsSlice)
	}
}

func decodeTotalZeros4x4Fast(br *BitReader, tableRow int) (int, bool) {
	if tableRow < 0 || tableRow >= 15 {
		return 0, false
	}
	peeked, err := br.PeekBits(totalZerosMaxPeek)
	if err != nil {
		return 0, false
	}
	e := totalZeros4x4LUT[tableRow][peeked]
	if !e.ok {
		return 0, false
	}
	br.SkipBits(e.codeLen)
	return e.value, true
}

func decodeChromaDCTotalZerosFast(br *BitReader, tableRow int) (int, bool) {
	if tableRow < 0 || tableRow >= 3 {
		return 0, false
	}
	peeked, err := br.PeekBits(3)
	if err != nil {
		return 0, false
	}
	e := chromaDCTotalZerosLUT[tableRow][peeked]
	if !e.ok {
		return 0, false
	}
	br.SkipBits(e.codeLen)
	return e.value, true
}

func decodeRunBeforeFast(br *BitReader, tableRow int) (int, bool) {
	if tableRow < 0 || tableRow >= 7 {
		return 0, false
	}
	peeked, err := br.PeekBits(runBeforeMaxPeek)
	if err != nil {
		return 0, false
	}
	e := runBeforeLUT[tableRow][peeked]
	if !e.ok {
		return 0, false
	}
	br.SkipBits(e.codeLen)
	return e.value, true
}
