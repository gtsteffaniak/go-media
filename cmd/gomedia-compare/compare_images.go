package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
	"github.com/gtsteffaniak/go-media/internal/rawpreview"
)

// appendImageCompareAssets builds HEIC/HEIF/RAW still fixtures from the reference
// clip frame so report thumbnails share the same source imagery as video fixtures.
//
// Coded HEVC-primary HEIC is not generated here: pinned ffmpeg 9.0.2 has no HEIF
// muxer (same as WMV3 encode). Add a checksum-pinned file under test/data when
// a coded-primary fixture is needed.
func appendImageCompareAssets(ffmpeg, clip, outDir string) ([]Asset, error) {
	dir := filepath.Join(outDir, "image")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	heicEmbed := filepath.Join(dir, "heic_jpeg_thumb_embed.jpg")
	heifEmbed := filepath.Join(dir, "heif_jpeg_thumb_embed.jpg")
	irotCoded := filepath.Join(dir, "heic_jpeg_irot_coded.jpg")
	irotDisplay := filepath.Join(dir, "heic_jpeg_irot_display.jpg")
	dngRef := filepath.Join(dir, "raw_dng_preview_embed.jpg")

	if err := seedFrameJPEG(ffmpeg, clip, heicEmbed, 320, 180); err != nil {
		return nil, fmt.Errorf("heic seed frame: %w", err)
	}
	if err := copyFile(heicEmbed, heifEmbed); err != nil {
		return nil, err
	}
	if err := seedFrameJPEG(ffmpeg, clip, irotCoded, 320, 180); err != nil {
		return nil, fmt.Errorf("heic irot coded frame: %w", err)
	}
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-i", irotCoded, "-frames:v", "1", "-vf", "transpose=2", "-q:v", "3", irotDisplay); err != nil {
		return nil, fmt.Errorf("heic irot display frame: %w", err)
	}
	if err := seedFrameJPEG(ffmpeg, clip, dngRef, 640, 360); err != nil {
		return nil, fmt.Errorf("dng seed frame: %w", err)
	}

	j320, err := os.ReadFile(heicEmbed)
	if err != nil {
		return nil, err
	}
	j640, err := os.ReadFile(dngRef)
	if err != nil {
		return nil, err
	}

	heicPath := filepath.Join(dir, "heic_jpeg_thumb.heic")
	heifPath := filepath.Join(dir, "heif_jpeg_thumb.heif")
	irotPath := filepath.Join(dir, "heic_jpeg_irot.heic")
	dngPath := filepath.Join(dir, "raw_dng_preview.dng")

	if err := os.WriteFile(heicPath, heifthumb.SynthJPEGHEIF(j320, 2, 1), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(heifPath, heifthumb.SynthJPEGHEIFMIF1(j320, 2, 1), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(irotPath, heifthumb.SynthJPEGHEIFWithPrimaryIrot(j320, 2, 1, 1), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(dngPath, rawpreview.SynthTIFFJPEG(j640), 0o644); err != nil {
		return nil, err
	}

	return []Asset{
		{ID: "heic_jpeg_thumb", Path: heicPath, Category: "image", Description: "HEIC embedded JPEG thumbnail"},
		{ID: "heif_jpeg_thumb", Path: heifPath, Category: "image", Description: "HEIF (mif1) embedded JPEG thumbnail"},
		{ID: "heic_jpeg_irot", Path: irotPath, Category: "image", Description: "HEIC embedded JPEG + primary irot (90° CCW)"},
		{ID: "raw_dng_preview", Path: dngPath, Category: "image", Description: "DNG-style TIFF embedded JPEG preview"},
	}, nil
}

// seedFrameJPEG writes one baseline 4:2:0 JPEG frame from the seed clip.
func seedFrameJPEG(ffmpeg, clip, out string, w, h int) error {
	return runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-ss", "2", "-i", clip, "-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:%d,format=yuvj420p", w, h), "-q:v", "3", out)
}
