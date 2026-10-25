package h264_test

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media"
	"github.com/gtsteffaniak/go-media/internal/isobmff"
	"github.com/gtsteffaniak/go-media/internal/ioat"
)

func baseline1080Path() string {
	if p := os.Getenv("GOMEDIA_BENCH_H264_BASELINE"); p != "" {
		return p
	}
	return "../../test/compare/harness/.assets/video/mp4_h264_baseline_1080.mp4"
}

// BenchmarkBaseline1080IDRDecode measures CAVLC IDR decode (demux + entropy + reconstruct, no scale/JPEG).
func BenchmarkBaseline1080IDRDecode(b *testing.B) {
	path := baseline1080Path()
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

// BenchmarkBaseline1080ThumbnailLarge is end-to-end thumbnail_large (decode + scale + JPEG) for stage vs gate comparison.
func BenchmarkBaseline1080ThumbnailLarge(b *testing.B) {
	path := baseline1080Path()
	f, err := os.Open(path)
	if err != nil {
		b.Skip(path)
	}
	defer f.Close()
	st, _ := f.Stat()
	opt := media.OptionsForPreset(media.ThumbnailPresetLarge, 0)
	opt.Seek = 10
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := media.Thumbnail(f, st.Size(), opt); err != nil {
			b.Fatal(err)
		}
	}
}
