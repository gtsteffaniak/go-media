package main

import (
	"testing"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

func TestThumbnailPresetMatchesMedia(t *testing.T) {
	for _, p := range thumbnailPresets {
		opt := media.OptionsForPreset(media.ThumbnailPreset(p.Name), 10)
		if opt.MaxWidth != p.MaxWidth || opt.MaxHeight != p.MaxHeight {
			t.Fatalf("%s: size go=%dx%d harness=%dx%d", p.Name, opt.MaxWidth, opt.MaxHeight, p.MaxWidth, p.MaxHeight)
		}
		if opt.Quality != p.Quality {
			t.Fatalf("%s: quality go=%d harness=%d", p.Name, opt.Quality, p.Quality)
		}
		if opt.Filter != media.ThumbnailFilterArea {
			t.Fatalf("%s: filter %v want area", p.Name, opt.Filter)
		}
	}
}

func TestFFmpegQScaleForPresets(t *testing.T) {
	if FFmpegJPEGQScale(76) != 9 {
		t.Fatalf("small q:v want 9 got %d", FFmpegJPEGQScale(76))
	}
	if FFmpegJPEGQScale(90) != 5 {
		t.Fatalf("large q:v want 5 got %d", FFmpegJPEGQScale(90))
	}
}
