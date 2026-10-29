package isobmff

import (
	"testing"
)

// aacTrak models two hours of 48 kHz AAC interleaved with video at ~0.5 s
// chunks whose sample counts alternate, so stsc has one entry per chunk.
func aacTrak() []byte {
	const frames = 337500 // 2 h * 48000 / 1024
	const chunks = 14400
	stsc := []uint32{}
	for c := 0; c < chunks; c++ {
		spc := uint32(23)
		if c%2 == 1 {
			spc = 24
		}
		stsc = append(stsc, uint32(c+1), spc, 1)
	}
	stco := make([]uint32, chunks)
	return synthSideTrak("soun", "mp4a",
		synthBox("stts", u32s(0, 1, frames, 1024)),
		synthBox("stsc", u32s(0, chunks), u32s(stsc...)),
		synthBox("stsz", u32s(0, 4, frames)),
		synthBox("stco", u32s(0, chunks), u32s(stco...)),
	)
}

// BenchmarkMoovIndexWithAudio indexes moov and reads one H.264 keyframe from
// a file whose audio trak has a large stsc, the uncached thumbnail path.
func BenchmarkMoovIndexWithAudio(b *testing.B) {
	data := buildSynthMP4(synthMP4Opts{Samples: 3600, GOP: 60, ExtraTraks: [][]byte{aacTrak()}})
	r := paddedReader{data: data, size: int64(len(data)) + 4*337500}
	b.ReportAllocs()
	for b.Loop() {
		idx, err := openMovieIndex(r, r.size)
		if err != nil {
			b.Fatal(err)
		}
		if _, ok := idx.h264SyncSample(1, 50); !ok {
			b.Fatal("no sample")
		}
	}
}
