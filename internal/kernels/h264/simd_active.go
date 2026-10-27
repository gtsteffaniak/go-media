package h264

// SIMDActive is true when experiment dispatch is on and at least one kernel is enabled.
func SIMDActive() bool {
	return KernelsUseSIMD && anyKernelSIMDEnabled()
}

// SIMDKernelsEnabled returns names of kernels with SIMD enabled in the flag map.
func SIMDKernelsEnabled() []string {
	var out []string
	for name, on := range simdKernelEnabled {
		if on && KernelsUseSIMD {
			out = append(out, name)
		}
	}
	return out
}
