package h264

// SIMDActive is true when experiment dispatch is on and at least one kernel is enabled.
func SIMDActive() bool {
	return KernelsUseSIMD && anyKernelSIMDEnabled()
}

// SIMDKernelsEnabled returns names of kernels with SIMD enabled by the static per-kernel flags.
func SIMDKernelsEnabled() []string {
	var out []string
	for _, k := range simdKernels {
		if k.on && KernelsUseSIMD {
			out = append(out, k.name)
		}
	}
	return out
}
