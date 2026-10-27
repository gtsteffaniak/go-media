package h264

// SetKernelSIMDForTest toggles a kernel flag (tests / benchmarks only).
func SetKernelSIMDForTest(name string, on bool) {
	simdKernelEnabled[name] = on
	refreshKernelsUseSIMD()
}
