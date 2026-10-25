package yuv

// Color represents a YCbCr color value in restricted/limited range.
type Color struct {
	Y, Cb, Cr uint8
}

func clipU8(v int) uint8 {
	return clampByte(v)
}

// RGBToYCbCr converts RGB to YCbCr using BT.601 limited range.
func RGBToYCbCr(r, g, b uint8) Color {
	return RGBToYCbCrCS(r, g, b, BT601, LimitedRange)
}
