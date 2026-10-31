package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"

	"github.com/gtsteffaniak/go-media/internal/heifthumb"
	"github.com/gtsteffaniak/go-media/internal/rawpreview"
)

func appendImageCompareAssets(outDir string) ([]Asset, error) {
	j320 := synthCompareJPEG(320, 240)
	heic := heifthumb.SynthJPEGHEIF(j320, 2, 1)
	dng := rawpreview.SynthTIFFJPEG(synthCompareJPEG(640, 480))

	heicPath := filepath.Join(outDir, "image/heic_jpeg_thumb.heic")
	dngPath := filepath.Join(outDir, "image/raw_dng_preview.dng")
	if err := os.MkdirAll(filepath.Dir(heicPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(heicPath, heic, 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(dngPath, dng, 0o644); err != nil {
		return nil, err
	}
	return []Asset{
		{ID: "heic_jpeg_thumb", Path: heicPath, Category: "video", Description: "HEIC embedded JPEG thumbnail"},
		{ID: "raw_dng_preview", Path: dngPath, Category: "video", Description: "DNG-style TIFF embedded JPEG preview"},
	}, nil
}

func synthCompareJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 2), uint8(y * 2), 40, 255})
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82})
	return buf.Bytes()
}
