package thumb

import (
	"os"
	"testing"
)

func BenchmarkMJPEGSmall(b *testing.B) {
	path := os.Getenv("GOMEDIA_BENCH_MJPEG")
	if path == "" {
		b.Skip("set GOMEDIA_BENCH_MJPEG")
	}
	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	st, _ := f.Stat()
	opt := Options{MaxWidth: 256, MaxHeight: 256, SeekPercent: 10, Quality: 76, Scaler: ScalerArea}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Generate(f, st.Size(), opt)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMJPEGLarge(b *testing.B) {
	path := os.Getenv("GOMEDIA_BENCH_MJPEG")
	if path == "" {
		b.Skip("set GOMEDIA_BENCH_MJPEG")
	}
	f, err := os.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	st, _ := f.Stat()
	opt := Options{MaxWidth: 1280, MaxHeight: 720, SeekPercent: 10, Quality: 90, Scaler: ScalerArea}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Generate(f, st.Size(), opt)
		if err != nil {
			b.Fatal(err)
		}
	}
}
