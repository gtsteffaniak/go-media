//go:build !goexperiment.simd || purego

package h264

func predictIntra4x4VerticalSIMD(dst []byte, top []byte) {
	predictIntra4x4VerticalPortable(dst, top)
}

func predictIntra4x4HorizontalSIMD(dst []byte, left []byte) {
	predictIntra4x4HorizontalPortable(dst, left)
}

func predictIntra16x16VerticalSIMD(dst []byte, top []byte) {
	predictIntra16x16VerticalPortable(dst, top)
}

func predictIntra16x16HorizontalSIMD(dst []byte, left []byte) {
	predictIntra16x16HorizontalPortable(dst, left)
}
