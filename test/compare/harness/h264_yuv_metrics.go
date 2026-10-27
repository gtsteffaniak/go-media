package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gtsteffaniak/go-media"
)

// h264DecodeYUVMetrics compares go-media IDR YUV420 to ffmpeg -skip_frame nokey yuv420p (decode-level gate).
func h264DecodeYUVMetrics(videoPath string, seekPercent float64) (psnr float64, ssim float64, err error) {
	f, err := openFileAt(videoPath)
	if err != nil {
		return 0, 0, err
	}
	// Match thumbnail decode (fast preview / skip deblock), aligned with ffmpeg -skip_loop_filter all on thumbs.
	goYUV, w, h, err := media.H264KeyframeYUV420(f, f.Size(), seekPercent, true)
	if err != nil {
		return 0, 0, err
	}
	dir := os.TempDir()
	goPath := filepath.Join(dir, "gomedia-h264-go.yuv")
	ffPath := filepath.Join(dir, "gomedia-h264-ff.yuv")
	if err := os.WriteFile(goPath, goYUV, 0o644); err != nil {
		return 0, 0, err
	}
	tSec, err := media.H264KeyframeTimeSec(f, f.Size(), seekPercent)
	if err != nil {
		return 0, 0, err
	}
	if err := ffmpegKeyframeYUV(videoPath, tSec, w, h, ffPath); err != nil {
		return 0, 0, err
	}
	psnr, err = rawYUVPSNR(goPath, ffPath, w, h)
	if err != nil {
		return 0, 0, err
	}
	if psnr > 100 || psnr < 0 {
		psnr = 100
	}
	ssim, err = rawYUVSSIM(goPath, ffPath, w, h)
	if ssim > 1 {
		ssim = 1
	}
	return psnr, ssim, err
}

func ffmpegKeyframeYUV(path string, syncSec float64, w, h int, dest string) error {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return err
	}
	size := fmt.Sprintf("%dx%d", w, h)
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-skip_frame", "nokey"}
	if syncSec > 0.001 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", syncSec))
	}
	args = append(args, "-i", path, "-frames:v", "1", "-s", size, "-pix_fmt", "yuv420p", dest)
	return exec.Command(ffmpeg, args...).Run()
}

func rawYUVPSNR(a, b string, w, h int) (float64, error) {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return 0, err
	}
	size := fmt.Sprintf("%dx%d", w, h)
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "info",
		"-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", size, "-i", a,
		"-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", size, "-i", b,
		"-lavfi", "psnr", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "PSNR") {
		return 0, fmt.Errorf("%w: %s", err, text)
	}
	return parsePSNRAverage(text)
}

func rawYUVSSIM(a, b string, w, h int) (float64, error) {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return 0, err
	}
	size := fmt.Sprintf("%dx%d", w, h)
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "info",
		"-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", size, "-i", a,
		"-f", "rawvideo", "-pix_fmt", "yuv420p", "-s", size, "-i", b,
		"-lavfi", "ssim", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "SSIM") {
		return 0, fmt.Errorf("%w: %s", err, text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "All:") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "All:") {
					val := strings.TrimPrefix(field, "All:")
					v, err := strconv.ParseFloat(val, 64)
					if err == nil {
						return v, nil
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("ssim not found")
}

func parsePSNRAverage(text string) (float64, error) {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "average:") && strings.Contains(line, "PSNR") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "average:") {
					v, err := strconv.ParseFloat(strings.TrimPrefix(field, "average:"), 64)
					if err == nil {
						if v >= 1e10 {
							return 100, nil
						}
						if v > 0 {
							return v, nil
						}
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("psnr not found")
}
