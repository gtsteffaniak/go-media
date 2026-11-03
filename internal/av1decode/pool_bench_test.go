package av1decode

import (
	"os"
	"path/filepath"
	"testing"
)

func readAV1(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "test", "data", "av1", name))
	if err != nil {
		t.Skip(name, err)
	}
	return b
}

func BenchmarkDecodeSampleCached(b *testing.B) {
	sample := readAV1(b, "tu_large.obu")
	if _, err := DecodeSampleCached(nil, sample); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := DecodeSampleCached(nil, sample); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeSampleCachedParallel(b *testing.B) {
	sample := readAV1(b, "tu_large.obu")
	if _, err := DecodeSampleCached(nil, sample); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := DecodeSampleCached(nil, sample); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
