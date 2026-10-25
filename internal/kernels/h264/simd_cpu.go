package h264

import "golang.org/x/sys/cpu"

// KernelsUseSIMD is true when this binary includes GOEXPERIMENT=simd kernels and the CPU has vector ISA.
var KernelsUseSIMD bool

func init() {
	KernelsUseSIMD = simdExperimentEnabled() && cpuHasSIMD()
}

func cpuHasSIMD() bool {
	return cpu.X86.HasAVX2 || cpu.ARM64.HasASIMD
}
