package thumb

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func BenchmarkMP4H264Baseline1080Thumb(b *testing.B) {
	path := os.Getenv("GOMEDIA_BENCH_H264_BASELINE")
	if path == "" {
		path = "../../test/compare/harness/.assets/video/mp4_h264_baseline_1080.mp4"
	}
	f, err := os.Open(path)
	if err != nil {
		b.Skip(path)
	}
	defer f.Close()
	st, _ := f.Stat()
	r := ioat.New(f, st.Size())
	opt := Options{MaxWidth: 320, MaxHeight: 180, SeekPercent: 10, Fast: true, Format: "jpeg"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Generate(r, st.Size(), opt)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMP4H264Baseline1080DecodeOnly(b *testing.B) {
	path := os.Getenv("GOMEDIA_BENCH_H264_BASELINE")
	if path == "" {
		path = "../../test/compare/harness/.assets/video/mp4_h264_baseline_1080.mp4"
	}
	f, err := os.Open(path)
	if err != nil {
		b.Skip(path)
	}
	defer f.Close()
	st, _ := f.Stat()
	r := ioat.New(f, st.Size())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := isobmff.H264ThumbnailYCbCr(r, st.Size(), 10, true); !ok {
			b.Fatal("decode")
		}
	}
}
