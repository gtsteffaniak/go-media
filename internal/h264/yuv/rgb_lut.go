package yuv

// RGBLUT holds per-component lookup tables for YCbCrToRGBCS. Summing the
// entries for one sample and shifting right by 8 gives the same pre-clamp
// value as YCbCrToRGBCS, so results are bit-exact while skipping the
// per-pixel coefficient switch and multiplies.
type RGBLUT struct {
	Y, CrR, CbG, CrG, CbB [256]int32
}

// NewRGBLUT builds the lookup tables for a colour space and range.
func NewRGBLUT(cs ColorSpace, rng Range) *RGBLUT {
	ic := getInvCoeffs(cs, rng)
	off := 16
	if rng == FullRange {
		off = 0
	}
	t := &RGBLUT{}
	for i := 0; i < 256; i++ {
		t.Y[i] = int32(ic.c_y*(i-off) + 128)
		c := i - 128
		t.CrR[i] = int32(ic.c_cr * c)
		t.CbG[i] = int32(ic.c_cb * c)
		t.CrG[i] = int32(ic.c_cr_g * c)
		t.CbB[i] = int32(ic.c_cb_b * c)
	}
	return t
}

// Clamp8 clamps a fixed-point (<<8) sum from RGBLUT to a byte.
func Clamp8(v int32) int32 {
	v >>= 8
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
