package main

import (
	"path/filepath"
	"strings"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// compareThumbnailOptions returns go-media options aligned with ffmpegThumbnailPreset for the same asset.
//
// Alignment (see test/compare/README.md):
//   - Preset max WxH, quality 1–100, area scaler: mirrored in thumbnail_preset.go ↔ media.ApplyPreset.
//   - H.264: seekPercent stays 10 (track sync pick); ffmpeg -ss uses H264KeyframeTimeSec(10).
//   - Other video: seekPercent tracks ffmpeg -ss = format duration × 10%.
//   - JPEG: go image/jpeg Quality vs ffmpeg -q:v via ffmpegQScale (MJPEG qscale, not identical tables).
func compareThumbnailOptions(videoPath string, preset thumbnailPresetConfig, keyframeOnly bool) media.ThumbnailOptions {
	opt := mediaPreset(preset)
	if keyframeOnly {
		return opt
	}
	seekSec := thumbnailSeekSeconds(videoPath, 10, false)
	if dur, err := ffprobeDuration(videoPath); err == nil && dur > 0 {
		opt.SeekPercent = (seekSec / dur) * 100.0
	}
	return opt
}

// keyframeOnlyForPath mirrors assetH264KeyframeOnly for subprocess benches (thumbmode).
func keyframeOnlyForPath(videoPath string) bool {
	id := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	if strings.Contains(id, "h264") || strings.HasPrefix(id, "mp4_h264_") {
		return true
	}
	switch id {
	case "mp4_sidecar_subs", "mp4_embedded_subs":
		return true
	}
	return false
}

// FFmpegJPEGQScale maps media thumbnail quality (1–100) to ffmpeg MJPEG -q:v (2–31).
func FFmpegJPEGQScale(quality int) int {
	return ffmpegQScale(quality)
}
