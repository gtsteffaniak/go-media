package isobmff

import (
	"bytes"
	"os"
	"testing"
)

// benchMP4 loads an MP4 named by env (GOMEDIA_BENCH_MP4_SHORT / _LONG) into memory.
func benchMP4(b *testing.B, env string) *bytes.Reader {
	b.Helper()
	p := os.Getenv(env)
	if p == "" {
		b.Skipf("%s not set", env)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		b.Skip(err)
	}
	return bytes.NewReader(data)
}

func benchSyncSample(b *testing.B, env string) {
	r := benchMP4(b, env)
	size := r.Size()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		track, ok := FirstAVCTrackIndex(r, size)
		if !ok {
			b.Fatal("no avc track")
		}
		if _, ok := H264SyncSample(r, size, track, 10); !ok {
			b.Fatal("no sync sample")
		}
	}
}

func BenchmarkH264SyncSampleShort(b *testing.B) { benchSyncSample(b, "GOMEDIA_BENCH_MP4_SHORT") }
func BenchmarkH264SyncSampleLong(b *testing.B)  { benchSyncSample(b, "GOMEDIA_BENCH_MP4_LONG") }

func benchThumbOpen(b *testing.B, env string) {
	r := benchMP4(b, env)
	size := r.Size()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok := loadH264Thumb(r, size, 10); !ok {
			b.Fatal("load failed")
		}
	}
}

func BenchmarkH264ThumbLoadShort(b *testing.B) { benchThumbOpen(b, "GOMEDIA_BENCH_MP4_SHORT") }
func BenchmarkH264ThumbLoadLong(b *testing.B)  { benchThumbOpen(b, "GOMEDIA_BENCH_MP4_LONG") }
