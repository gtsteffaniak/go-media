package bench

// Micro benchmarks for public helpers that take no media input. They are
// cheap, so they are here to catch accidental regressions (for example an
// allocation added to an error constructor), not to rank hot paths.

import (
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func BenchmarkAllThumbnailFilters(b *testing.B) {
	for b.Loop() {
		sinkAny = media.AllThumbnailFilters()
	}
}

func BenchmarkThumbnailFilterLabel(b *testing.B) {
	filters := media.AllThumbnailFilters()
	for b.Loop() {
		for _, f := range filters {
			sinkStr = media.ThumbnailFilterLabel(f)
		}
	}
}

func BenchmarkSIMDActive(b *testing.B) {
	for b.Loop() {
		sinkBool = media.SIMDActive()
	}
}

func BenchmarkOptionsForPreset(b *testing.B) {
	for b.Loop() {
		sinkAny = media.OptionsForPreset(media.ThumbnailPresetLarge, 10)
	}
}

func BenchmarkApplyPreset(b *testing.B) {
	for b.Loop() {
		var opt media.ThumbnailOptions
		media.ApplyPreset(&opt, media.ThumbnailPresetSmall)
		sinkAny = opt
	}
}

func BenchmarkUnsupportedError(b *testing.B) {
	for b.Loop() {
		sinkAny = media.Unsupported("thumbnail", "hevc")
	}
}
