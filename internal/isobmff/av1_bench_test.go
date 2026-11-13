package isobmff_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/av1decode"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
)

func BenchmarkMP4AV1Thumbnail(b *testing.B) {
	path := filepath.Join("..", "..", "test", "data", "av1", "s8_1080.mp4")
	f, err := os.Open(path)
	if err != nil {
		b.Skip(err)
	}
	defer f.Close()
	st, _ := f.Stat()
	src, err := isobmff.OpenThumbSourceErr(f, st.Size())
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := src.AV1ThumbnailYCbCr(10, av1decode.FrameLimits{}); err != nil {
			b.Fatal(err)
		}
	}
}
