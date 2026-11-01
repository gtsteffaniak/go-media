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
func appendImageCompareAssets(ffmpeg, hevcEnc, clip, outDir string) ([]Asset, error) {
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

	var assets []Asset
	assets = append(assets,
		Asset{ID: "heic_jpeg_thumb", Path: heicPath, Category: "image", Description: "HEIC embedded JPEG thumbnail"},
		Asset{ID: "heif_jpeg_thumb", Path: heifPath, Category: "image", Description: "HEIF (mif1) embedded JPEG thumbnail"},
		Asset{ID: "heic_jpeg_irot", Path: irotPath, Category: "image", Description: "HEIC embedded JPEG + primary irot (90° CCW)"},
		Asset{ID: "raw_dng_preview", Path: dngPath, Category: "image", Description: "DNG-style TIFF embedded JPEG preview"},
	)

	coded, err := generateHEICCodedStill(ffmpeg, hevcEnc, clip, dir)
	if err != nil {
		return nil, fmt.Errorf("heic/heif coded still fixtures: %w", err)
	}
	assets = append(assets, coded...)

	return assets, nil
}

func generateHEICCodedStill(ffmpeg, hevcEnc, clip, dir string) ([]Asset, error) {
	_, muxFmt, ok := ffmpegHEIFContainer(ffmpeg)
	if !ok {
		return nil, fmt.Errorf("ffmpeg has no HEIC/HEIF demuxer")
	}
	heicPath := filepath.Join(dir, "heic_coded.heic")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-ss", "2", "-i", clip, "-frames:v", "1", "-vf", "scale=480:-2",
		"-c:v", hevcEnc, "-tag:v", "hvc1", "-f", muxFmt, heicPath); err != nil {
		return nil, err
	}
	out := []Asset{
		{ID: "heic_coded", Path: heicPath, Category: "image", Description: "HEIC HEVC primary still (ffmpeg mux)"},
	}
	heifPath := filepath.Join(dir, "heif_coded.heif")
	if err := runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-ss", "2", "-i", clip, "-frames:v", "1", "-vf", "scale=480:-2",
		"-c:v", hevcEnc, "-tag:v", "hvc1", "-f", muxFmt, heifPath); err != nil {
		if err := copyFile(heicPath, heifPath); err != nil {
			return out, nil
		}
	}
	out = append(out, Asset{ID: "heif_coded", Path: heifPath, Category: "image", Description: "HEIF HEVC primary still (ffmpeg mux)"})
	return out, nil
}

// seedFrameJPEG writes one baseline 4:2:0 JPEG frame from the seed clip.
func seedFrameJPEG(ffmpeg, clip, out string, w, h int) error {
	return runFF(ffmpeg, "-y", "-hide_banner", "-loglevel", "error",
		"-ss", "2", "-i", clip, "-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:%d,format=yuvj420p", w, h), "-q:v", "3", out)
}
