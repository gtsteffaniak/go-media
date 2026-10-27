package thumb

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func BenchmarkMP4H264Large(b *testing.B) {
	benchH264Thumb(b, "../../test/compare/harness/.assets/video/mp4_h264.mp4", h264BenchLargeOpt())
}

func BenchmarkMP4H264Small(b *testing.B) {
	benchH264Thumb(b, "../../test/compare/harness/.assets/video/mp4_h264.mp4", h264BenchSmallOpt())
}

func BenchmarkMP4H264Baseline1080Small(b *testing.B) {
	benchH264Thumb(b, "../../test/compare/harness/.assets/video/mp4_h264_baseline_1080.mp4", h264BenchSmallOpt())
}

func BenchmarkMP4H264Baseline1080Large(b *testing.B) {
	benchH264Thumb(b, "../../test/compare/harness/.assets/video/mp4_h264_baseline_1080.mp4", h264BenchLargeOpt())
}

func BenchmarkMP4H264Main1080Large(b *testing.B) {
	benchH264Thumb(b, "../../test/compare/harness/.assets/video/mp4_h264_main_1080.mp4", h264BenchLargeOpt())
}

func BenchmarkMP4H264High1080Large(b *testing.B) {
	benchH264Thumb(b, "../../test/compare/harness/.assets/video/mp4_h264_high_1080.mp4", h264BenchLargeOpt())
}

func h264BenchLargeOpt() Options {
	return Options{MaxWidth: 1280, MaxHeight: 720, SeekPercent: 10, Quality: 90, Format: "jpeg", Scaler: ScalerArea}
}

func h264BenchSmallOpt() Options {
	return Options{
		MaxWidth: 256, MaxHeight: 256, SeekPercent: 10, Quality: 76,
		Format: "jpeg", Scaler: ScalerArea, Fast: true,
	}
}

func benchH264Thumb(b *testing.B, path string, opt Options) {
	f, err := os.Open(path)
	if err != nil {
		b.Skip(path)
	}
	defer f.Close()
	st, _ := f.Stat()
	r := ioat.New(f, st.Size())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Generate(r, st.Size(), opt); err != nil {
			b.Fatal(err)
		}
	}
}
