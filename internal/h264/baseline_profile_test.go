package h264

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/abema/go-mp4"
	"github.com/gtsteffaniak/go-media/internal/h264/cleanroom"
)

// BenchmarkBaselineThumbnailDecode profiles CAVLC decode for mp4_h264_baseline_1080
// (bypasses validateSPSNAL rejection). Feeds docs/BASELINE_PROFILE.md until cleanroom ships.
func BenchmarkBaselineThumbnailDecode(b *testing.B) {
	avcc, sample := openBaselineKeyframe(b)
	data := prependParameterSets(avcc, sample)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dec := borrowDecoder(true)
		frames, err := dec.DecodeIDRAVC(data)
		releaseDecoder(dec)
		if err != nil || len(frames) == 0 {
			b.Fatal(err)
		}
		_ = cleanroom.ToFrame(frames[0])
	}
}

func BenchmarkBaselineThumbnailDecodeFullDeblock(b *testing.B) {
	avcc, sample := openBaselineKeyframe(b)
	data := prependParameterSets(avcc, sample)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dec := borrowDecoder(false)
		frames, err := dec.DecodeIDRAVC(data)
		releaseDecoder(dec)
		if err != nil || len(frames) == 0 {
			b.Fatal(err)
		}
	}
}

func openBaselineKeyframe(b *testing.B) (*mp4.AVCDecoderConfiguration, []byte) {
	path := filepath.Join("..", "..", "test", "compare", "harness", ".assets", "video", "mp4_h264_baseline_1080.mp4")
	f, err := os.Open(path)
	if err != nil {
		b.Skip(err)
	}
	st, _ := f.Stat()
	avcc, sample, err := firstSyncSample(f, st.Size())
	if err != nil {
		b.Fatal(err)
	}
	return avcc, sample
}
