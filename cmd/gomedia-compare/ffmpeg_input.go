package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// ffmpegThumbInput returns a path ffmpeg can decode for thumbnail extraction.
// Pinned ffmpeg 9.0.2 segfaults on MPEG-TS demux in this environment; we demux
// with go-media and pass elementary video. Minimal synth HEIC containers use
// display-aligned sidecar JPEGs for synth HEIC/HEIF fixtures.
func ffmpegThumbInput(assetPath, assetID string) (input string, cleanup func(), err error) {
	cleanup = func() {}
	if sidecar, ok := stillImageFFmpegSidecar(assetPath, assetID); ok {
		if _, err := os.Stat(sidecar); err != nil {
			return "", cleanup, fmt.Errorf("still image sidecar %s: %w", assetID, err)
		}
		return sidecar, cleanup, nil
	}
	if isStillImageAsset(assetID) {
		return assetPath, cleanup, nil
	}
	if !tsLikeContainer(assetPath) {
		return assetPath, cleanup, nil
	}
	f, err := openFileAt(assetPath)
	if err != nil {
		return "", cleanup, err
	}
	size := f.Size()
	es, codec, err := media.MPEGTSVideoElementary(f, size)
	if err != nil {
		return "", cleanup, err
	}
	ext := ".m2v"
	if strings.Contains(codec, "264") {
		ext = ".h264"
	}
	tmp, err := os.CreateTemp("", "gomedia-ts-es-*"+ext)
	if err != nil {
		return "", cleanup, err
	}
	cleanup = func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(es); err != nil {
		tmp.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmp.Name(), cleanup, nil
}

func stillImageFFmpegSidecar(assetPath, assetID string) (string, bool) {
	dir := filepath.Dir(assetPath)
	switch assetID {
	case "heic_jpeg_thumb":
		return filepath.Join(dir, "heic_jpeg_thumb_embed.jpg"), true
	case "heif_jpeg_thumb":
		return filepath.Join(dir, "heif_jpeg_thumb_embed.jpg"), true
	case "heic_jpeg_irot":
		return filepath.Join(dir, "heic_jpeg_irot_display.jpg"), true
	case "heic_phone_primary":
		return filepath.Join(dir, "heic_phone_primary_display.png"), true
	case "raw_dng_preview":
		return filepath.Join(dir, "raw_dng_preview_embed.jpg"), true
	default:
		return "", false
	}
}

// ffmpegScaleViaFilterComplex is true when ffmpeg's demuxer already uses a complex
// filtergraph (HEIC/HEIF/AVIF containers), so -vf cannot be combined with decode.
func ffmpegScaleViaFilterComplex(assetPath, assetID string) bool {
	if _, ok := stillImageFFmpegSidecar(assetPath, assetID); ok {
		return false
	}
	switch strings.ToLower(filepath.Ext(assetPath)) {
	case ".heic", ".heif", ".avif":
		return true
	default:
		return false
	}
}
