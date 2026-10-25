package nal

// EBSPtoRBSPDst writes de-emulated RBSP into dst and returns the length used.
// dst must have length at least len(ebsp) (RBSP never grows).
func EBSPtoRBSPDst(dst []byte, ebsp []byte) int {
	if len(dst) < len(ebsp) {
		panic("nal: EBSPtoRBSPDst buffer too small")
	}
	w := 0
	zeros := 0
	for i := 0; i < len(ebsp); i++ {
		b := ebsp[i]
		if zeros >= 2 && b == 0x03 && (i+1 >= len(ebsp) || ebsp[i+1] <= 0x03) {
			zeros = 0
			continue
		}
		dst[w] = b
		w++
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return w
}

// EBSPtoRBSP removes emulation prevention bytes (allocates once).
func EBSPtoRBSP(ebsp []byte) []byte {
	dst := make([]byte, len(ebsp))
	n := EBSPtoRBSPDst(dst, ebsp)
	return dst[:n]
}
