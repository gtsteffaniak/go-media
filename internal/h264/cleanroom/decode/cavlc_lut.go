package decode

import "github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"

// Prefix LUTs for canonical CAVLC codewords (replaces per-decode map lookups).

const cavlcPeekBits = 16

type ctSlot struct {
	bits uint8
	t0   int8
	tc   int8
}

type vlcSlot struct {
	bits uint8
	val  int8
}

func buildCTLUT(entries []ctEntry) [1 << cavlcPeekBits]ctSlot {
	var lut [1 << cavlcPeekBits]ctSlot
	for _, e := range entries {
		L := int(e.length)
		C := int(e.code)
		shift := cavlcPeekBits - L
		hi := (C << shift) + (1 << shift)
		for k := C << shift; k < hi; k++ {
			lut[k] = ctSlot{bits: uint8(L), t0: e.t0, tc: e.tc}
		}
	}
	return lut
}

func buildVLCLUT(entries []vlcEntry) [1 << cavlcPeekBits]vlcSlot {
	var lut [1 << cavlcPeekBits]vlcSlot
	for _, e := range entries {
		L := int(e.length)
		C := int(e.code)
		shift := cavlcPeekBits - L
		hi := (C << shift) + (1 << shift)
		for k := C << shift; k < hi; k++ {
			lut[k] = vlcSlot{bits: uint8(L), val: e.val}
		}
	}
	return lut
}

func decodeCoeffTokenLUT(r *bitstream.Reader, lut *[1 << cavlcPeekBits]ctSlot) (int, int) {
	peek := r.PeekBits(cavlcPeekBits)
	e := lut[peek]
	if e.bits == 0 {
		return 0, 0
	}
	r.Skip(int(e.bits))
	return int(e.t0), int(e.tc)
}

func decodeVLCLUT(r *bitstream.Reader, lut *[1 << cavlcPeekBits]vlcSlot) int {
	peek := r.PeekBits(cavlcPeekBits)
	e := lut[peek]
	if e.bits == 0 {
		return 0
	}
	r.Skip(int(e.bits))
	return int(e.val)
}

// buildVLCLUTN expands entries into a prefix LUT of width w bits (w ≥ max code length).
func buildVLCLUTN(entries []vlcEntry, w int, lut []vlcSlot) {
	for _, e := range entries {
		L := int(e.length)
		shift := w - L
		lo := int(e.code) << shift
		for k := lo; k < lo+(1<<shift); k++ {
			lut[k] = vlcSlot{bits: uint8(L), val: e.val}
		}
	}
}

const (
	tzPeekBits = 9
	rbPeekBits = 3
)

var (
	tzSmall       [16][1 << tzPeekBits]vlcSlot
	tzChromaSmall [4][1 << rbPeekBits]vlcSlot
	rbSmall       [7][1 << rbPeekBits]vlcSlot
)

func init() {
	for i := range totalZerosTab {
		buildVLCLUTN(totalZerosTab[i], tzPeekBits, tzSmall[i][:])
	}
	for i := range totalZerosChromaTab {
		buildVLCLUTN(totalZerosChromaTab[i], rbPeekBits, tzChromaSmall[i][:])
	}
	for i := range runBeforeCanon {
		buildVLCLUTN(runBeforeCanon[i], rbPeekBits, rbSmall[i][:])
	}
}

func decodeVLCSmall(r *bitstream.Reader, lut []vlcSlot, w int) int {
	e := lut[r.PeekBits(w)&(1<<w-1)]
	if e.bits == 0 {
		return 0
	}
	r.Skip(int(e.bits))
	return int(e.val)
}
