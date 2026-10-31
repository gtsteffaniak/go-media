package vc1

import "sync"

const (
	mbIntraBits = 9
	dcVLCBits   = 9
)

type lutEntry struct {
	len uint8
	val int16
}

var (
	vlcOnce     sync.Once
	mbIntraLUT  [1 << mbIntraBits]lutEntry
	dcLumaLUT   [2][1 << dcVLCBits]lutEntry
	dcChromaLUT [2][1 << dcVLCBits]lutEntry
)

func init() {
	vlcOnce.Do(initVLCLUTs)
}

func initVLCLUTs() {
	fillDC(0, dcLuma0[:], dcLumaLUT[0][:])
	fillDC(0, dcLuma1[:], dcLumaLUT[1][:])
	fillDC(0, dcChroma0[:], dcChromaLUT[0][:])
	fillDC(0, dcChroma1[:], dcChromaLUT[1][:])
	for i, e := range mbIntraCBP {
		fillLUT(mbIntraLUT[:], mbIntraBits, e.code, e.bits, int16(i))
	}
}

func fillDC(_ int, tab []dcEntry, lut []lutEntry) {
	for i, e := range tab {
		fillLUT(lut, dcVLCBits, e.code, e.bits, int16(i))
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

func decode012(r *bitReader) int {
	if !r.bit() {
		return 0
	}
	if !r.bit() {
		return 1
	}
	return 2
}
