package h264

import "testing"

// Layer 1: DC semantics and dequant saturation — see dc_semantics_test.go, dequant_saturation_test.go.

// TestLayer2IDCTAdd16Smoke exercises the batched luma path (Layer 2 / compare-gate companion).
func TestLayer2IDCTAdd16Smoke(t *testing.T) {
	var plane [256]byte
	var pred [256]byte
	var blocks [16][16]int32
	var nz [16]uint8
	blocks[0][0] = 96
	blocks[3][1] = 2
	nz[3] = 1
	for i := 0; i < 200; i++ {
		IDCTAdd16Luma(plane[:], 16, 0, 0, pred[:], 16, &blocks, nz[:])
	}
}
