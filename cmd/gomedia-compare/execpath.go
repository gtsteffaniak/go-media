package main

import (
	"fmt"
	"os"
	"os/exec"
)

// ffmpegPaths returns ffmpeg and ffprobe binaries from PATH.
func ffmpegPaths() (ffmpeg, ffprobe string, err error) {
	ffmpeg, err = lookFFmpeg()
	if err != nil {
		return "", "", err
	}
	ffprobe, err = lookFFprobe()
	if err != nil {
		return "", "", err
	}
	return ffmpeg, ffprobe, nil
}

// lookFFmpeg locates the ffmpeg binary.
func lookFFmpeg() (string, error) {
	if p := os.Getenv("FFMPEG_PATH"); p != "" {
		return p, nil
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found in PATH (install via scripts/install-ffmpeg.sh)")
	}
	return p, nil
}

// lookFFprobe locates the ffprobe binary.
func lookFFprobe() (string, error) {
	if p := os.Getenv("FFPROBE_PATH"); p != "" {
		return p, nil
	}
	p, err := exec.LookPath("ffprobe")
	if err != nil {
		return "", fmt.Errorf("ffprobe not found in PATH")
	}
	return p, nil
}
