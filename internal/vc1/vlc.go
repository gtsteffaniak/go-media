package vc1

import "sync"

const (
	mbIntraBits    = 9
	mbIntraMaxBits = 13
	dcVLCBits      = 9
)

type lutEntry struct {
	len uint8
	val int16
}

var (
	vlcOnce        sync.Once
	mbIntraSorted  []struct{ code uint32; bits uint8; val uint8 }
	mbIntraLUT    [1 << mbIntraBits]lutEntry
	mbIntraLUT13  [1 << mbIntraMaxBits]lutEntry
	dcLumaLUT    [2][1 << dcVLCBits]lutEntry
	dcChromaLUT  [2][1 << dcVLCBits]lutEntry
	acCoeffLUT   [8][1 << acVLCBits]lutEntry
	acVLCSorted  [8][]acVLCEntry
	dcLumaSorted [2][]dcEntry
	dcChromaSorted [2][]dcEntry
)

func init() {
	vlcOnce.Do(initVLCLUTs)
}

func initVLCLUTs() {
	fillDC(dcLuma0[:], dcLumaLUT[0][:])
	fillDC(dcLuma1[:], dcLumaLUT[1][:])
	fillDC(dcChroma0[:], dcChromaLUT[0][:])
	fillDC(dcChroma1[:], dcChromaLUT[1][:])
	dcLumaSorted[0] = append([]dcEntry(nil), dcLuma0[:]...)
	dcLumaSorted[1] = append([]dcEntry(nil), dcLuma1[:]...)
	dcChromaSorted[0] = append([]dcEntry(nil), dcChroma0[:]...)
	dcChromaSorted[1] = append([]dcEntry(nil), dcChroma1[:]...)
	for i := range dcLumaSorted {
		sortDCVLC(dcLumaSorted[i])
		sortDCVLC(dcChromaSorted[i])
	}
	for i, e := range mbIntraCBP {
		fillLUT(mbIntraLUT[:], mbIntraBits, e.code, e.bits, int16(i))
		fillLUT(mbIntraLUT13[:], mbIntraMaxBits, e.code, e.bits, int16(i))
	}
	mbIntraSorted = append(mbIntraSorted[:0], mbIntraCBP[:]...)
	for i := 1; i < len(mbIntraSorted); i++ {
		e := mbIntraSorted[i]
		j := i - 1
		for j >= 0 && mbIntraSorted[j].bits < e.bits {
			mbIntraSorted[j+1] = mbIntraSorted[j]
			j--
		}
		mbIntraSorted[j+1] = e
	}
	for set := 0; set < 8; set++ {
		acVLCSorted[set] = append([]acVLCEntry(nil), acVLCTables[set]...)
		sortACVLC(acVLCSorted[set])
		for _, e := range acVLCSorted[set] {
			if int(e.bits) <= acVLCBits {
				fillLUTFirst(acCoeffLUT[set][:], acVLCBits, e.code, e.bits, int16(e.idx))
			}
		}
	}
}

func sortACVLC(tab []acVLCEntry) {
	for i := 1; i < len(tab); i++ {
		e := tab[i]
		j := i - 1
		for j >= 0 && tab[j].bits < e.bits {
			tab[j+1] = tab[j]
			j--
		}
		tab[j+1] = e
	}
}

func sortDCVLC(tab []dcEntry) {
	for i := 1; i < len(tab); i++ {
		e := tab[i]
		j := i - 1
		for j >= 0 && tab[j].bits < e.bits {
			tab[j+1] = tab[j]
			j--
		}
		tab[j+1] = e
	}
}

func fillDC(tab []dcEntry, lut []lutEntry) {
	for i, e := range tab {
		fillLUT(lut, dcVLCBits, e.code, e.bits, int16(i))
	}
}

func fillLUTFirst(lut []lutEntry, bits int, code uint32, n uint8, val int16) {
	if n == 0 || int(n) > bits {
		return
	}
	shift := bits - int(n)
	base := int(code) << shift
	for i := 0; i < 1<<shift; i++ {
		if lut[base+i].len == 0 {
			lut[base+i] = lutEntry{len: n, val: val}
		}
	}
}

func fillLUT(lut []lutEntry, bits int, code uint32, n uint8, val int16) {
	if n == 0 || int(n) > bits {
		return
	}
	shift := bits - int(n)
	base := int(code) << shift
	for i := 0; i < 1<<shift; i++ {
		lut[base+i] = lutEntry{len: n, val: val}
	}
}

func (r *bitReader) vlc9(lut []lutEntry, bits int) (int16, bool) {
	if r.left() < bits {
		return 0, false
	}
	w := int(r.peek(bits))
	e := lut[w]
	if e.len == 0 {
		return 0, false
	}
	r.skip(int(e.len))
	return e.val, true
}

func (r *bitReader) vlcAC(set int) (int, bool) {
	if r.left() < acVLCBits {
		return 0, false
	}
	w := int(r.peek(acVLCBits))
	e := acCoeffLUT[set][w]
	if e.len != 0 {
		r.skip(int(e.len))
		return int(e.val), true
	}
	for _, ent := range acVLCSorted[set] {
		nb := int(ent.bits)
		if nb <= acVLCBits || r.left() < nb {
			continue
		}
		if r.peek(nb) == ent.code {
			r.skip(nb)
			return ent.idx, true
		}
	}
	return 0, false
}

func (r *bitReader) vlcDC(chroma bool, table int) (int16, bool) {
	tab := dcLumaSorted[table]
	if chroma {
		tab = dcChromaSorted[table]
	}
	for _, ent := range tab {
		nb := int(ent.bits)
		if nb == 0 || r.left() < nb {
			continue
		}
		if r.peek(nb) == ent.code {
			r.skip(nb)
			return ent.val, true
		}
	}
	return 0, false
}

func (r *bitReader) vlcMBIntra() (int, bool) {
	for _, ent := range mbIntraSorted {
		nb := int(ent.bits)
		if nb == 0 || r.left() < nb {
			continue
		}
		if r.peek(nb) == ent.code {
			r.skip(nb)
			return int(ent.val), true
		}
	}
	return 0, false
}

func esc3UnaryLevelLen(r *bitReader) int {
	n := 0
	for n < 6 && !r.bit() {
		n++
	}
	return n + 2
}

func decode012(r *bitReader) int {
	if !r.bit() {
		return 0
	}
	if !r.bit() {
		return 1
	}
	return 2
}

func decode210(r *bitReader) int {
	if r.bit() {
		return 0
	}
	if r.bit() {
		return 1
	}
	return 2
}

func (r *bitReader) decodeAcCoeff(set int, esc3LevelLen, esc3RunLen *int, pq int, dquantfrm bool) (last bool, skip, value int, ok bool) {
	idx, ok := r.vlcAC(set)
	if !ok {
		return false, 0, 0, false
	}
	lastIdx := acLastIndex[set]
	size := acTableSizes[set]
	if idx != size-1 {
		e := acIndexDecode[set][idx]
		skip = int(e.skip)
		last = idx >= lastIdx || r.left() < 0
		v := int(e.level)
		sign := r.bit()
		if sign {
			v = -v
		}
		return last, skip, v, true
	}
	return r.decodeAcCoeffEscape(set, esc3LevelLen, esc3RunLen, pq, dquantfrm)
}

func (r *bitReader) decodeAcCoeffEscape(set int, esc3LevelLen, esc3RunLen *int, pq int, dquantfrm bool) (last bool, skip, value int, ok bool) {
	lastIdx := acLastIndex[set]
	size := acTableSizes[set]
	escape := decode210(r)
	if escape != 2 {
		idx, ok := r.vlcAC(set)
		if !ok || idx >= size-1 {
			return false, 0, 0, false
		}
		e := acIndexDecode[set][idx]
		run := int(e.skip)
		level := int(e.level)
		last = idx >= lastIdx
		if escape == 0 {
			if last {
				level += int(acLastDeltaLevel[set][run])
			} else {
				level += int(acDeltaLevel[set][run])
			}
		} else {
			if last {
				run += int(acLastDeltaRun[set][level]) + 1
			} else {
				run += int(acDeltaRun[set][level]) + 1
			}
		}
		sign := r.bit()
		if sign {
			level = -level
		}
		return last, run, level, true
	}
	last = r.bit()
	if *esc3LevelLen == 0 {
		if pq < 8 || dquantfrm {
			n := int(r.bits(3))
			if n == 0 {
				n = int(r.bits(2)) + 8
			}
			*esc3LevelLen = n
		} else {
			*esc3LevelLen = esc3UnaryLevelLen(r)
		}
		*esc3RunLen = 3 + int(r.bits(2))
	}
	run := int(r.bits(*esc3RunLen))
	sign := r.bit()
	level := int(r.bits(*esc3LevelLen))
	if sign {
		level = -level
	}
	return last, run, level, true
}
