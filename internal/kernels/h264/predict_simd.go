//go:build goexperiment.simd

package h264

import "simd"

func predictIntra4x4VerticalSIMD(dst []byte, top []byte) {
	var row [4]int32
	for i := 0; i < 4; i++ {
		row[i] = int32(top[i])
	}
	v, _ := simd.LoadInt32sPart(row[:])
	for y := 0; y < 4; y++ {
		v.StorePart(row[:])
		for x := 0; x < 4; x++ {
			dst[y*4+x] = byte(row[x])
		}
	}
}

func predictIntra4x4HorizontalSIMD(dst []byte, left []byte) {
	for y := 0; y < 4; y++ {
		b := simd.BroadcastInt32s(int32(left[y]))
		var out [4]int32
		b.StorePart(out[:])
		for x := 0; x < 4; x++ {
			dst[y*4+x] = byte(out[x])
		}
	}
}

func predictIntra16x16VerticalSIMD(dst []byte, top []byte) {
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x += 4 {
			var row [4]int32
			for i := 0; i < 4; i++ {
				row[i] = int32(top[x+i])
			}
			v, _ := simd.LoadInt32sPart(row[:])
			v.StorePart(row[:])
			for i := 0; i < 4; i++ {
				dst[y*16+x+i] = byte(row[i])
			}
		}
	}
}

func predictIntra16x16HorizontalSIMD(dst []byte, left []byte) {
	for y := 0; y < 16; y++ {
		b := simd.BroadcastInt32s(int32(left[y]))
		for x := 0; x < 16; x += 4 {
			var out [4]int32
			b.StorePart(out[:])
			for i := 0; i < 4; i++ {
				dst[y*16+x+i] = byte(out[i])
			}
		}
	}
}
