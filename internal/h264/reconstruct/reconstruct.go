package reconstruct

// ReconstructLuma4x4 writes pred+residual into yPlane at (px,py), clipping to [0,255].
func ReconstructLuma4x4(yPlane []byte, stride int, px, py int, pred [4][4]uint8, residual [16]int32) {
	reconstructLuma4x4(yPlane, stride, px, py, pred, residual)
}
