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
// display-aligned sidecar JPEGs; ffmpeg-generated heic_coded/heif_coded use the
// container path directly.
func ffmpegThumbInput(assetPath, assetID string) (input string, cleanup func(), err error) {
	cleanup = func() {}
	if sidecar, ok := stillImageFFmpegSidecar(assetPath, assetID); ok {
		if _, err := os.Stat(sidecar); err != nil {
			return "", cleanup, fmt.Errorf("still image sidecar %s: %w", assetID, err)
		}
		return sidecar, cleanup, nil
	}
	if isStillImageAsset(assetID) && stillImageUsesContainer(assetID) {
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

func stillImageUsesContainer(assetID string) bool {
	switch assetID {
	case "heic_coded", "heif_coded":
		return true
	default:
		return false
	}
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
	case "raw_dng_preview":
		return filepath.Join(dir, "raw_dng_preview_embed.jpg"), true
	default:
		return "", false
	}
}
