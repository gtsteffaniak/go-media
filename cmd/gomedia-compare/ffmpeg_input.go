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
// with go-media and pass elementary video. HEIC/DNG fixtures use sidecar JPEGs
// (embedded preview source) because ffmpeg cannot read our minimal synth containers.
func ffmpegThumbInput(assetPath, assetID string) (input string, cleanup func(), err error) {
	cleanup = func() {}
	switch assetID {
	case "heic_jpeg_thumb":
		ref := filepath.Join(filepath.Dir(assetPath), "heic_jpeg_thumb_embed.jpg")
		if _, err := os.Stat(ref); err != nil {
			return "", cleanup, fmt.Errorf("heic embed ref: %w", err)
		}
		return ref, cleanup, nil
	case "raw_dng_preview":
		ref := filepath.Join(filepath.Dir(assetPath), "raw_dng_preview_embed.jpg")
		if _, err := os.Stat(ref); err != nil {
			return "", cleanup, fmt.Errorf("dng embed ref: %w", err)
		}
		return ref, cleanup, nil
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
