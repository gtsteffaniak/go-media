package h264

// PredictIntra4x4Vertical fills a 4×4 block from top neighbors.
func PredictIntra4x4Vertical(dst []byte, top []byte) {
	predictIntra4x4VerticalPortable(dst, top)
}

// PredictIntra4x4Horizontal fills a 4×4 block from left neighbors.
func PredictIntra4x4Horizontal(dst []byte, left []byte) {
	predictIntra4x4HorizontalPortable(dst, left)
}

// PredictIntra16x16Vertical copies the top row across a 16×16 luma prediction block.
func PredictIntra16x16Vertical(dst []byte, top []byte) {
	predictIntra16x16VerticalPortable(dst, top)
}

// PredictIntra16x16Horizontal copies the left column across each row.
func PredictIntra16x16Horizontal(dst []byte, left []byte) {
	predictIntra16x16HorizontalPortable(dst, left)
}

func predictIntra4x4VerticalPortable(dst []byte, top []byte) {
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			dst[y*4+x] = top[x]
		}
	}
}

func predictIntra4x4HorizontalPortable(dst []byte, left []byte) {
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			dst[y*4+x] = left[y]
		}
	}
}

func predictIntra16x16VerticalPortable(dst []byte, top []byte) {
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			dst[y*16+x] = top[x]
		}
	}
}

func predictIntra16x16HorizontalPortable(dst []byte, left []byte) {
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			dst[y*16+x] = left[y]
		}
	}
}
