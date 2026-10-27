package h264

import "testing"

func BenchmarkIDCTAdd16Luma(b *testing.B) {
	var plane [256]byte
	var pred [256]byte
	for i := range pred {
		pred[i] = 128
	}
	var blocks [16][16]int32
	var nz [16]uint8
	blocks[0][0] = 96
	blocks[5][1] = 4
	nz[5] = 1
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		IDCTAdd16Luma(plane[:], 16, 0, 0, pred[:], 16, &blocks, nz[:])
	}
}
