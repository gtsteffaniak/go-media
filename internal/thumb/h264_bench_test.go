package thumb

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func BenchmarkMP4H264Large(b *testing.B) {
	path := "../../test/compare/harness/.assets/video/mp4_h264.mp4"
	f, err := os.Open(path)
	if err != nil {
		b.Skip(path)
	}
	defer f.Close()
	st, _ := f.Stat()
	r := ioat.New(f, st.Size())
	opt := Options{MaxWidth: 1280, MaxHeight: 720, SeekPercent: 10, Quality: 90, Format: "jpeg"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Generate(r, st.Size(), opt); err != nil {
			b.Fatal(err)
		}
	}
}

