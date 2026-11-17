package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHEICDumpOutput writes the JPEG output for each file in
// GOMEDIA_HEIC_BENCH_FILES to GOMEDIA_HEIC_DUMP_DIR, so output can be compared
// before and after a change. Skipped unless both variables are set.
func TestHEICDumpOutput(t *testing.T) {
	list := os.Getenv("GOMEDIA_HEIC_BENCH_FILES")
	dir := os.Getenv("GOMEDIA_HEIC_DUMP_DIR")
	if list == "" || dir == "" {
		t.Skip("GOMEDIA_HEIC_BENCH_FILES and GOMEDIA_HEIC_DUMP_DIR not set")
	}
	opt := ThumbnailOptions{MaxWidth: 16384, MaxHeight: 16384, Format: "jpeg", Quality: 80, MaxSourcePixels: 4096 * 4096}
	for _, p := range strings.Split(list, ",") {
		p = strings.TrimSpace(p)
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		st, _ := f.Stat()
		res, err := Thumbnail(f, st.Size(), opt)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out := filepath.Join(dir, filepath.Base(p)+".jpg")
		if err := os.WriteFile(out, res.Data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// BenchmarkHEICThumbnail times the full-resolution HEIC JPEG decode filebrowser
// requests (heicDecodeMaxEdge bounds, 4096x4096 source pixel cap). Run it with
// a comma-separated list of files:
//
//	GOMEDIA_HEIC_BENCH_FILES=a.heic,b.HEIC go test ./pkg/media -run '^$' -bench HEICThumbnail -cpu 1 -benchtime 5x
//
// It is skipped when the variable is unset, so CI never needs sample files.
func BenchmarkHEICThumbnail(b *testing.B) {
	list := os.Getenv("GOMEDIA_HEIC_BENCH_FILES")
	if list == "" {
		b.Skip("GOMEDIA_HEIC_BENCH_FILES not set")
	}
	opt := ThumbnailOptions{
		MaxWidth:        16384,
		MaxHeight:       16384,
		Format:          "jpeg",
		Quality:         80,
		MaxSourcePixels: 4096 * 4096,
	}
	for _, p := range strings.Split(list, ",") {
		p = strings.TrimSpace(p)
		b.Run(filepath.Base(p), func(b *testing.B) {
			f, err := os.Open(p)
			if err != nil {
				b.Fatal(err)
			}
			defer f.Close()
			st, err := f.Stat()
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Thumbnail(f, st.Size(), opt); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
