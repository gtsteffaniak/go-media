//go:build goexperiment.simd

package h264

import (
	"testing"
)

func BenchmarkIDCT4x4Portable(b *testing.B) {
	var d [16]int32
	d[0] = 96
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inverseTransform4x4ResidualPortable(d)
	}
}

func BenchmarkIDCT4x4SIMD(b *testing.B) {
	SetSIMDEnabledForTest(true)
	defer ResetSIMDTestHooks()
	if !KernelsUseSIMD {
		b.Skip("no AVX2/ASIMD or SIMD opt-in off")
	}
	var d [16]int32
	d[0] = 96
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inverseTransform4x4ResidualSIMD(d)
	}
}
