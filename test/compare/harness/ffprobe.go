package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gtsteffaniak/go-media"
)

// FFProbeFormat holds ffprobe format fields used for comparison.
type FFProbeFormat struct {
	FormatName string
	Duration   float64
	Tags       map[string]string
}

// FFProbeStream is a simplified stream entry from ffprobe JSON.
type FFProbeStream struct {
	Index        int
	CodecType    string
	CodecName    string
	Width        int
	Height       int
	SampleRate   string
	Channels     int
	AttachedPic  int
}

// FFProbeResult aggregates ffprobe output for one file.
type FFProbeResult struct {
	Format  FFProbeFormat
	Streams []FFProbeStream
}

// probeFFprobe runs ffprobe and parses JSON output.
func probeFFprobe(path string) (FFProbeResult, error) {
	ffprobe, err := lookFFprobe()
	if err != nil {
		return FFProbeResult{}, err
	}
	out, err := exec.Command(ffprobe,
		"-v", "error",
		"-show_entries", "format=format_name,duration:stream=index,codec_type,codec_name,width,height,sample_rate,channels:stream_disposition=attached_pic",
		"-show_entries", "format_tags=title,artist,album,date,genre",
		"-of", "json",
		path,
	).Output()
	if err != nil {
		return FFProbeResult{}, err
	}
	var raw struct {
		Format struct {
			FormatName string            `json:"format_name"`
			Duration   string            `json:"duration"`
			Tags       map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			Index      int    `json:"index"`
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
			Disposition struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return FFProbeResult{}, err
	}
	dur, _ := strconv.ParseFloat(raw.Format.Duration, 64)
	res := FFProbeResult{
		Format: FFProbeFormat{
			FormatName: raw.Format.FormatName,
			Duration:   dur,
			Tags:       raw.Format.Tags,
		},
	}
	for _, s := range raw.Streams {
		res.Streams = append(res.Streams, FFProbeStream{
			Index: s.Index, CodecType: s.CodecType, CodecName: s.CodecName,
			Width: s.Width, Height: s.Height, SampleRate: s.SampleRate, Channels: s.Channels,
			AttachedPic: s.Disposition.AttachedPic,
		})
	}
	return res, nil
}

// ffprobeDuration returns duration in seconds.
func ffprobeDuration(path string) (float64, error) {
	p, err := probeFFprobe(path)
	if err != nil {
		return 0, err
	}
	return p.Format.Duration, nil
}

// ffmpegVersionLine returns the first line of ffmpeg -version.
func ffmpegVersionLine() string {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return ""
	}
	out, err := exec.Command(ffmpeg, "-version").Output()
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(out), "\n", 2)
	return strings.TrimSpace(lines[0])
}

// ffprobeVersionLine returns the first line of ffprobe -version.
func ffprobeVersionLine() string {
	ffprobe, err := lookFFprobe()
	if err != nil {
		return ""
	}
	out, err := exec.Command(ffprobe, "-version").Output()
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(out), "\n", 2)
	return strings.TrimSpace(lines[0])
}

// thumbnailSeekSeconds returns ffmpeg -ss time aligned with go-media thumbnail seek.
func thumbnailSeekSeconds(videoPath string, seekPercent float64, keyframeOnly bool) float64 {
	if keyframeOnly {
		f, err := openFileAt(videoPath)
		if err != nil {
			return 0
		}
		sec, err := media.H264KeyframeTimeSec(f, f.Size(), seekPercent)
		if err != nil {
			return 0
		}
		return sec
	}
	dur, err := ffprobeDuration(videoPath)
	if err != nil || dur <= 0 {
		dur = 5
	}
	return dur * seekPercent / 100.0
}

// ffmpegThumbnailPreset extracts one JPEG frame aligned with a media thumbnail preset.
// seekSec is presentation time in seconds (use stss sync time for H.264 intra thumbnails).
// When keyframeOnly is true, ffmpeg decodes only IDR/I frames (matches go-media H.264 intra path).
func ffmpegThumbScaleFlags(preset thumbnailPresetConfig, keyframeOnly bool) string {
	// go-media H.264 intra thumbnails use area downscale in YCbCr (ThumbnailFilterArea).
	if keyframeOnly {
		return "flags=area"
	}
	return preset.FFScale
}

func ffmpegThumbnailPreset(path, dest string, seekSec float64, preset thumbnailPresetConfig, keyframeOnly bool) error {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return err
	}
	scaleFlags := ffmpegThumbScaleFlags(preset, keyframeOnly)
	vf := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:%s,scale=trunc(iw/2)*2:trunc(ih/2)*2",
		preset.MaxWidth, preset.MaxHeight, scaleFlags)
	qv := ffmpegQScale(preset.Quality)
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if keyframeOnly {
		args = append(args, "-skip_frame", "nokey")
	}
	if seekSec > 0.001 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
	}
	args = append(args, "-i", path, "-frames:v", "1", "-vf", vf, "-q:v", fmt.Sprintf("%d", qv), dest)
	return exec.Command(ffmpeg, args...).Run()
}

// ffmpegRasterPNG writes a lossless PNG frame using the same seek/scale as thumbnails.
func ffmpegRasterPNG(path, dest string, seekSec float64, preset thumbnailPresetConfig, keyframeOnly bool) error {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return err
	}
	scaleFlags := ffmpegThumbScaleFlags(preset, keyframeOnly)
	vf := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:%s,scale=trunc(iw/2)*2:trunc(ih/2)*2",
		preset.MaxWidth, preset.MaxHeight, scaleFlags)
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if keyframeOnly {
		args = append(args, "-skip_frame", "nokey")
	}
	if seekSec > 0.001 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
	}
	args = append(args, "-i", path, "-frames:v", "1", "-vf", vf, dest)
	return exec.Command(ffmpeg, args...).Run()
}

func ffmpegQScale(quality int) int {
	if quality < 1 {
		quality = 1
	}
	if quality > 100 {
		quality = 100
	}
	q := 31 - (quality*29)/100
	if q < 2 {
		return 2
	}
	if q > 31 {
		return 31
	}
	return q
}

// imagePSNRSSIM compares two image files with separate ffmpeg psnr and ssim filters.
func imagePSNRSSIM(a, b string) (psnr float64, ssim float64, err error) {
	psnr, err = imagePSNR(a, b)
	if err != nil {
		return 0, 0, err
	}
	ssim, err = imageSSIM(a, b)
	if err != nil {
		return psnr, 0, err
	}
	return psnr, ssim, nil
}

func imagePSNR(a, b string) (float64, error) {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "info", "-i", a, "-i", b, "-lavfi", "psnr", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "PSNR") && !strings.Contains(text, "psnr") {
		return 0, fmt.Errorf("%w: %s", err, text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "psnr_avg:") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "psnr_avg:") {
					v, _ := strconv.ParseFloat(strings.TrimPrefix(field, "psnr_avg:"), 64)
					if v > 0 {
						return v, nil
					}
				}
			}
		}
		if strings.Contains(line, "average:") && strings.Contains(line, "PSNR") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "average:") {
					v, _ := strconv.ParseFloat(strings.TrimPrefix(field, "average:"), 64)
					if v > 0 {
						return v, nil
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("psnr not found in ffmpeg output: %s", text)
}

func imageSSIM(a, b string) (float64, error) {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return 0, err
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "info", "-i", a, "-i", b, "-lavfi", "ssim", "-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "SSIM") {
		return 0, fmt.Errorf("%w: %s", err, text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "All:") && strings.Contains(line, "SSIM") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "All:") {
					val := strings.TrimPrefix(field, "All:")
					if val == "inf" || val == "(inf)" {
						continue
					}
					v, err := strconv.ParseFloat(val, 64)
					if err == nil {
						return v, nil
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("ssim not found in ffmpeg output: %s", text)
}
