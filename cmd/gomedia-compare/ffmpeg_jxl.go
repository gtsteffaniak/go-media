package main

import (
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	jxlDecodeProbeMu sync.Mutex
	jxlDecodeProbe   map[string]bool
)

// ffmpegCannotReferenceJXL is true for .jxl fixtures when the ffmpeg in use cannot decode
// JPEG XL (no libjxl). Those assets are checked on go-media alone; no ffmpeg reference is made.
func ffmpegCannotReferenceJXL(assetPath string) bool {
	return strings.EqualFold(filepath.Ext(assetPath), ".jxl") && !ffmpegCanDecodeJPEGXL(assetPath)
}

// ffmpegCanDecodeJPEGXL reports whether ffmpeg can decode path (libjxl / jpegxl decoder).
func ffmpegCanDecodeJPEGXL(path string) bool {
	jxlDecodeProbeMu.Lock()
	defer jxlDecodeProbeMu.Unlock()
	if jxlDecodeProbe == nil {
		jxlDecodeProbe = map[string]bool{}
	}
	if v, ok := jxlDecodeProbe[path]; ok {
		return v
	}
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		jxlDecodeProbe[path] = false
		return false
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-i", path, "-frames:v", "1", "-f", "null", "-")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	ok := cmd.Run() == nil
	jxlDecodeProbe[path] = ok
	return ok
}
