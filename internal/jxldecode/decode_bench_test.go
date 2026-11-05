package jxldecode_test

import (
	"os"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/jxldecode"
)

// BenchmarkDecodeOversized measures rejecting a 6000x4000 header (24 MP) in a
// 64 MiB file for a thumbnail box under a 4096x4096 caller cap; the probe avoids reading the file.
func BenchmarkDecodeOversized(b *testing.B) {
	const size = 64 << 20
	pr := &paddedReader{head: syntheticHeader(6000, 4000), size: size}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := jxldecode.DecodeWithOptions(pr, size, 256, 256, jxldecode.Options{MaxPixels: 4096 * 4096}); err == nil {
			b.Fatal("expected error")
		}
	}
}

// BenchmarkDecodeSmall measures the probe overhead on a normal photo.
func BenchmarkDecodeSmall(b *testing.B) {
	data, err := os.ReadFile(testDataPath(referenceJXL))
	if err != nil {
		b.Skip(err)
	}
	pr := &paddedReader{head: data, size: int64(len(data))}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := jxldecode.Decode(pr, pr.size, 256, 256); err != nil {
			b.Fatal(err)
		}
	}
}
