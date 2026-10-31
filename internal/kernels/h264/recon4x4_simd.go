//go:build goexperiment.simd

package h264

import "simd"

func writeBlock4x4SIMD(plane []byte, stride, px0, py0 int, pred []byte, predStride, lx, ly int, res [16]int32) {
	zero := simd.BroadcastInt32s(0)
	max255 := simd.BroadcastInt32s(255)
	for yy := 0; yy < 4; yy++ {
		row := (py0 + yy) * stride
		poff := (ly + yy) * predStride
		rv, _ := simd.LoadInt32sPart(res[yy*4 : yy*4+4])
		p4 := [4]int32{
			int32(pred[poff+lx+0]),
			int32(pred[poff+lx+1]),
			int32(pred[poff+lx+2]),
			int32(pred[poff+lx+3]),
		}
		pv, _ := simd.LoadInt32sPart(p4[:])
		sum := pv.Add(rv)
		sum = sum.Max(zero).Min(max255)
		var out [4]int32
		sum.StorePart(out[:])
		for xx := 0; xx < 4; xx++ {
			plane[row+px0+xx] = byte(out[xx])
		}
	}
}
