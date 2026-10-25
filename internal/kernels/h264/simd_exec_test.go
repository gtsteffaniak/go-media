package h264

import (
	"os"
	"testing"
)

func TestSIMDIDCTParity(t *testing.T) {
	cases := [][16]int32{
		{96, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{1, -1, 2, 0, 3, 4, -2, 1, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	for _, in := range cases {
		want := inverseTransform4x4ResidualPortable(in)
		got := InverseTransform4x4Residual(in)
		for i := 0; i < 16; i++ {
			if got[i] != want[i] {
				t.Fatalf("coeff %d: got %d want %d (simd=%v)", i, got[i], want[i], KernelsUseSIMD)
			}
		}
	}
}

func TestSIMDKernelsExecuteWhenEnabled(t *testing.T) {
	if os.Getenv("GOEXPERIMENT") == "" && !simdExperimentEnabled() {
		t.Skip("build with GOEXPERIMENT=simd to assert SIMD execution")
	}
	SetSIMDEnabledForTest(true)
	SetKernelSIMDForTest("idct4x4", true)
	SetKernelSIMDForTest("recon4x4", true)
	if !KernelSIMD("idct4x4") {
		t.Skip("CPU lacks AVX2/ASIMD or simd experiment disabled")
	}
	ResetSIMDProbe()
	var d [16]int32
	d[0] = 96
	_ = InverseTransform4x4Residual(d)
	var plane [64]byte
	var pred [256]byte
	var res [16]int32
	res[0] = 1
	WriteBlock4x4(plane[:], 8, 0, 0, pred[:], 16, 0, 0, res)
	if SIMDProbeHits() < 2 {
		t.Fatalf("expected SIMD probe hits >=2, got %d", SIMDProbeHits())
	}
}
