package thumb_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtsteffaniak/go-media/internal/thumb"
)

// BenchmarkThumbnailHEICPhone measures the pinned HEVC-primary phone HEIC,
// which exercises the meta walk, irot/imir parse and coded-primary decode.
func BenchmarkThumbnailHEICPhone(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "data", "reference_soundboard.heic"))
	if err != nil {
		b.Skip(err)
	}
	r := bytes.NewReader(data)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := thumb.Generate(r, int64(len(data)), thumb.Options{MaxWidth: 256, MaxHeight: 256}); err != nil {
			b.Fatal(err)
		}
	}
}
