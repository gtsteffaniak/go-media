package h264

import "testing"

func randPair(seed *uint32, amp int32) (a, b [16]int32) {
	next := func() int32 {
		*seed = *seed*1664525 + 1013904223
		return int32(*seed>>16)%(2*amp+1) - amp
	}
	for i := range a {
		a[i], b[i] = next(), next()
	}
	return
}

func TestIDCTAdd4x4PairMatchesPortable(t *testing.T) {
	seed := uint32(7)
	for iter := 0; iter < 20000; iter++ {
		amp := int32(64)
		if iter%3 == 0 {
			amp = 4096 // exercises clipping in both directions
		}
		a, b := randPair(&seed, amp)
		const stride = 24
		var got, want [stride * 5]byte
		for i := range got {
			seed = seed*1664525 + 1013904223
			got[i] = byte(seed >> 24)
			want[i] = got[i]
		}
		off := stride + 3
		idctAdd4x4PairSIMD(got[:], off, stride, &a, &b)
		idctAdd4x4PairPortable(want[:], off, stride, &a, &b)
		if got != want {
			t.Fatalf("iter %d mismatch\na=%v\nb=%v\ngot=%v\nwant=%v", iter, a, b, got, want)
		}
	}
}

func benchPair(b *testing.B, fn func([]byte, int, int, *[16]int32, *[16]int32)) {
	seed := uint32(3)
	var blocks [64][2][16]int32
	for i := range blocks {
		blocks[i][0], blocks[i][1] = randPair(&seed, 96)
	}
	plane := make([]byte, 64*16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &blocks[i&63]
		fn(plane, (i&7)*8, 64, &p[0], &p[1])
	}
}

func BenchmarkIDCTAdd4x4PairPortable(b *testing.B) { benchPair(b, idctAdd4x4PairPortable) }
func BenchmarkIDCTAdd4x4PairSIMD(b *testing.B)     { benchPair(b, idctAdd4x4PairSIMD) }

func TestIDCTAdd4x4QuadMatchesPortable(t *testing.T) {
	if !archHasAVX512() {
		t.Skip("no AVX-512")
	}
	seed := uint32(11)
	for iter := 0; iter < 20000; iter++ {
		amp := int32(64)
		if iter%3 == 0 {
			amp = 4096
		}
		var q [4][16]int32
		q[0], q[1] = randPair(&seed, amp)
		q[2], q[3] = randPair(&seed, amp)
		const stride = 40
		var got, want [stride * 5]byte
		for i := range got {
			seed = seed*1664525 + 1013904223
			got[i] = byte(seed >> 24)
			want[i] = got[i]
		}
		off := stride + 5
		idctAdd4x4QuadAVX512(got[:], off, stride, &q)
		idctAdd4x4QuadPortable(want[:], off, stride, &q)
		if got != want {
			t.Fatalf("iter %d mismatch", iter)
		}
	}
}

func benchQuad(b *testing.B, fn func([]byte, int, int, *[4][16]int32)) {
	seed := uint32(3)
	var blocks [32][4][16]int32
	for i := range blocks {
		blocks[i][0], blocks[i][1] = randPair(&seed, 96)
		blocks[i][2], blocks[i][3] = randPair(&seed, 96)
	}
	plane := make([]byte, 64*16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn(plane, (i&3)*16, 64, &blocks[i&31])
	}
}

func BenchmarkIDCTAdd4x4QuadPortable(b *testing.B) { benchQuad(b, idctAdd4x4QuadPortable) }
func BenchmarkIDCTAdd4x4QuadAVX2Pairs(b *testing.B) {
	benchQuad(b, func(p []byte, off, stride int, q *[4][16]int32) {
		idctAdd4x4PairSIMD(p, off, stride, &q[0], &q[1])
		idctAdd4x4PairSIMD(p, off+8, stride, &q[2], &q[3])
	})
}
func BenchmarkIDCTAdd4x4QuadAVX512(b *testing.B) {
	if !archHasAVX512() {
		b.Skip("no AVX-512")
	}
	benchQuad(b, idctAdd4x4QuadAVX512)
}

func TestIDCTAdd4x4BlockMatchesPortable(t *testing.T) {
	seed := uint32(11)
	for iter := 0; iter < 20000; iter++ {
		amp := int32(64)
		if iter%3 == 0 {
			amp = 4096
		}
		d, _ := randPair(&seed, amp)
		const stride, pstride = 24, 16
		var got, want [stride * 6]byte
		var pred [pstride * 5]byte
		for i := range got {
			seed = seed*1664525 + 1013904223
			got[i] = byte(seed >> 24)
			want[i] = got[i]
		}
		for i := range pred {
			seed = seed*1664525 + 1013904223
			pred[i] = byte(seed >> 24)
		}
		off, poff := stride+5, pstride+2
		idctAdd4x4BlockSIMD(got[:], off, stride, pred[:], poff, pstride, &d)
		idctAdd4x4BlockPortable(want[:], off, stride, pred[:], poff, pstride, &d)
		if got != want {
			t.Fatalf("iter %d mismatch d=%v", iter, d)
		}
	}
}

func TestIDCTDCAdd4x4MatchesTransform(t *testing.T) {
	var pred [64]byte
	for i := range pred {
		pred[i] = byte(i * 37)
	}
	for dc := int32(-9000); dc <= 9000; dc += 7 {
		var got, want [8 * 8]byte
		IDCTDCAdd4x4(got[:], 8, 2, 1, pred[:], 8, 1, 2, dc)
		var d [16]int32
		d[0] = dc
		res := inverseTransform4x4ResidualPortable(d)
		writeBlock4x4Portable(want[:], 8, 2, 1, pred[:], 8, 1, 2, res)
		if got != want {
			t.Fatalf("dc=%d mismatch", dc)
		}
	}
}

func benchBlock(b *testing.B, fn func([]byte, int, int, []byte, int, int, *[16]int32)) {
	seed := uint32(3)
	var blocks [64][16]int32
	for i := range blocks {
		blocks[i], _ = randPair(&seed, 96)
	}
	plane := make([]byte, 64*16)
	pred := make([]byte, 16*16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn(plane, (i&7)*4, 64, pred, (i&3)*4, 16, &blocks[i&63])
	}
}

func BenchmarkIDCTAdd4x4BlockPortable(b *testing.B) { benchBlock(b, idctAdd4x4BlockPortable) }
func BenchmarkIDCTAdd4x4BlockSIMD(b *testing.B)     { benchBlock(b, idctAdd4x4BlockSIMD) }
