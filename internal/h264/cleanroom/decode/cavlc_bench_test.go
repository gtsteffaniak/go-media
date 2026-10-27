package decode

import (
	"testing"

	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom/bitstream"
)

func benchCAVLCReader() *bitstream.Reader {
	// MB0 I_16x16 DC-only style block with 8 levels (synthetic).
	bits := "00100" + "1" + "1" + "000101" + "1" + "1" + "1"
	return bitsReader(bits)
}

func BenchmarkResidualBlockCAVLC(b *testing.B) {
	r := benchCAVLCReader()
	var sd sliceDecoder
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r = benchCAVLCReader()
		_ = sd.residualBlockCAVLC(r, 0, 16)
	}
}

func BenchmarkReadLevelsInto(b *testing.B) {
	bits := "1" + "0" + "1" + "000101" + "1" + "1" + "1"
	r := bitsReader(bits)
	var level [16]int32
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r = bitsReader(bits)
		readLevelsInto(r, 3, 8, level[:8])
	}
}
