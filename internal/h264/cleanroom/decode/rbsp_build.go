package decode

// bitWriter is a minimal bitstream encoder for building controlled RBSP in tests.
type bitWriter struct{ bits []byte }

func (w *bitWriter) u(n int, v uint32) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, byte((v>>uint(i))&1))
	}
}
func (w *bitWriter) flag(b bool) {
	if b {
		w.u(1, 1)
	} else {
		w.u(1, 0)
	}
}
func bitLen(v uint32) int {
	n := 0
	for v > 0 {
		n++
		v >>= 1
	}
	return n
}
func (w *bitWriter) ue(v uint32) {
	x := v + 1
	nbits := bitLen(x)
	for i := 0; i < nbits-1; i++ {
		w.bits = append(w.bits, 0)
	}
	w.u(nbits, x)
}
func (w *bitWriter) se(v int32) {
	var k uint32
	if v > 0 {
		k = uint32(2*v - 1)
	} else {
		k = uint32(-2 * v)
	}
	w.ue(k)
}
func (w *bitWriter) bytes() []byte {
	out := make([]byte, (len(w.bits)+7)/8)
	for i, b := range w.bits {
		if b == 1 {
			out[i>>3] |= 1 << uint(7-(i&7))
		}
	}
	return out
}

// BaselineSPSRBSP builds an SPS RBSP in ParseSPS order. cropBottom is applied when
// frame_cropping_flag is set (for heights not divisible by 16). high=true adds the
// chroma/bit_depth block.
func BaselineSPSRBSP(profile, level uint8, widthMbsM1, heightMapM1, cropBottom uint32, high bool) []byte {
	return encodeSPSOpts(profile, level, 0, widthMbsM1, heightMapM1, cropBottom, high, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
}

func encodeSPS(profile, level uint8, widthMbsM1, heightMapM1, cropBottom uint32, high bool) []byte {
	return BaselineSPSRBSP(profile, level, widthMbsM1, heightMapM1, cropBottom, high)
}

func encodeSPSOpts(profile, level uint8, seqID, widthMbsM1, heightMapM1, cropBottom uint32, high bool, log2FN, pocType, log2POC, pocCycle uint32, chromaIDC, bdLuma, bdChroma uint32, cropL, cropR, cropT, cropB uint32) []byte {
	w := &bitWriter{}
	w.u(8, uint32(profile))
	w.u(8, 0)
	w.u(8, uint32(level))
	w.ue(seqID)
	if high {
		if chromaIDC == 0 {
			chromaIDC = 1
		}
		w.ue(chromaIDC)
		if chromaIDC == 3 {
			w.flag(false)
		}
		w.ue(bdLuma)
		w.ue(bdChroma)
		w.flag(false)
		w.flag(false)
	}
	w.ue(log2FN)
	w.ue(pocType)
	switch pocType {
	case 0:
		w.ue(log2POC)
	case 1:
		w.flag(false)
		w.se(0)
		w.se(0)
		w.ue(pocCycle)
		for i := uint32(0); i < pocCycle; i++ {
			w.se(0)
		}
	}
	w.ue(1)
	w.flag(false)
	w.ue(widthMbsM1)
	w.ue(heightMapM1)
	w.flag(true)
	w.flag(true)
	if cropBottom > 0 || cropL > 0 || cropR > 0 || cropT > 0 || cropB > 0 {
		w.flag(true)
		w.ue(cropL)
		w.ue(cropR)
		w.ue(cropT)
		if cropB > 0 {
			w.ue(cropB)
		} else {
			w.ue(cropBottom)
		}
	} else {
		w.flag(false)
	}
	return w.bytes()
}

// BaselinePPSRBSP builds a minimal PPS RBSP (CAVLC unless cabac is true).
func (w *bitWriter) align() {
	for len(w.bits)%8 != 0 {
		w.bits = append(w.bits, 0)
	}
}

// BuildIPCMSliceRBSP builds an IDR I_PCM slice RBSP for a 1×1 macroblock frame.
func BuildIPCMSliceRBSP(sps *SPS, ppsID uint32) []byte {
	w := &bitWriter{}
	w.ue(0)
	w.ue(uint32(SliceI))
	w.ue(ppsID)
	w.u(sps.Log2MaxFrameNum, 0)
	w.ue(0)
	w.u(sps.Log2MaxPocLsb, 0)
	w.flag(false)
	w.flag(false)
	w.se(0)
	w.ue(25)
	w.align()
	for i := 0; i < 256; i++ {
		w.u(8, uint32(i))
	}
	for i := 0; i < 64; i++ {
		w.u(8, uint32(i))
	}
	for i := 0; i < 64; i++ {
		w.u(8, uint32(63-i))
	}
	w.u(8, 0x80)
	return w.bytes()
}

func BaselinePPSRBSP(ppsID, spsID uint32, cabac bool) []byte {
	w := &bitWriter{}
	w.ue(ppsID)
	w.ue(spsID)
	w.flag(cabac)
	w.flag(false)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.flag(false)
	w.u(2, 0)
	w.se(0)
	w.se(0)
	w.se(0)
	w.flag(false)
	w.flag(false)
	w.flag(false)
	return w.bytes()
}
