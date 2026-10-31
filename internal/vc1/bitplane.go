package vc1

func bitplaneDecode(r *bitReader, width, height, stride int) int {
	mode := decode012(r)
	switch mode {
	case 0:
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				_ = r.bit()
			}
		}
	case 1:
		for i := 0; i < width*height; i++ {
			_ = r.bit()
		}
	case 2:
		_ = r.bit()
		for i := 0; i < width*height; i++ {
			_ = r.bit()
		}
	}
	_ = stride
	return 0
}
