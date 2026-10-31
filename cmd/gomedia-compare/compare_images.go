package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
	"github.com/gtsteffaniak/go-media/internal/rawpreview"
)

// appendImageCompareAssets builds the HEIC and DNG fixtures from a Big Buck
// Bunny frame (the same seed clip as every video fixture), so all report
// thumbnails show the same source.
func appendImageCompareAssets(ffmpeg, clip, outDir string) ([]Asset, error) {
	dir := filepath.Join(outDir, "image")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	heicRef := filepath.Join(dir, "heic_jpeg_thumb_embed.jpg")
	dngRef := filepath.Join(dir, "raw_dng_preview_embed.jpg")
	if err := seedFrameJPEG(ffmpeg, clip, heicRef, 320, 180); err != nil {
		return nil, fmt.Errorf("heic seed frame: %w", err)
	}
	if err := seedFrameJPEG(ffmpeg, clip, dngRef, 640, 360); err != nil {
		return nil, fmt.Errorf("dng seed frame: %w", err)
	}
	j320, err := os.ReadFile(heicRef)
	if err != nil {
		return nil, err
	}
	j640, err := os.ReadFile(dngRef)
	if err != nil {
		return nil, err
	}
	heicPath := filepath.Join(dir, "heic_jpeg_thumb.heic")
	dngPath := filepath.Join(dir, "raw_dng_preview.dng")
	if err := os.WriteFile(heicPath, heifthumb.SynthJPEGHEIF(j320, 2, 1), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(dngPath, rawpreview.SynthTIFFJPEG(j640), 0o644); err != nil {
		return nil, err
	}
	return []Asset{
		{ID: "heic_jpeg_thumb", Path: heicPath, Category: "video", Description: "HEIC embedded JPEG thumbnail"},
		{ID: "raw_dng_preview", Path: dngPath, Category: "video", Description: "DNG-style TIFF embedded JPEG preview"},
	}, nil
}

// seedFrameJPEG writes one baseline 4:2:0 JPEG frame from the seed clip.
func seedFrameJPEG(ffmpeg, clip, out string, w, h int) error {
	return runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-ss", "2", "-i", clip, "-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:%d,format=yuvj420p", w, h), "-q:v", "3", out)
}
