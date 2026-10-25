package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

// ffmpegThumbnail extracts one JPEG frame at seekPercent into dest.
func ffmpegThumbnail(path, dest string, seekPercent float64) error {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return err
	}
	dur, err := ffprobeDuration(path)
	if err != nil || dur <= 0 {
		dur = 5
	}
	ss := dur * seekPercent / 100.0
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if dur >= 0.5 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", ss))
	}
	args = append(args, "-i", path, "-frames:v", "1", "-vf", "scale='min(320,iw)':-2", "-q:v", "2", dest)
	return exec.Command(ffmpeg, args...).Run()
}

// imagePSNRSSIM compares two PNG/JPEG files via ffmpeg lavfi.
func imagePSNRSSIM(a, b string) (psnr float64, ssim float64, err error) {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return 0, 0, err
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-i", a, "-i", b,
		"-lavfi", "[0:v][1:v]scale2ref[ref][main];[main][ref]psnr=stats_file=-;[main][ref]ssim=stats_file=-",
		"-f", "null", "-")
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil && !strings.Contains(text, "PSNR") {
		return 0, 0, fmt.Errorf("%w: %s", err, text)
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "psnr_avg:") {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "psnr_avg:") {
					psnr, _ = strconv.ParseFloat(strings.TrimPrefix(field, "psnr_avg:"), 64)
				}
			}
		}
		if strings.Contains(line, "PSNR") && strings.Contains(line, "average:") {
			fields := strings.Fields(line)
			for i, f := range fields {
				if f == "average:" && i+1 < len(fields) {
					psnr, _ = strconv.ParseFloat(strings.TrimSuffix(fields[i+1], "dB"), 64)
				}
			}
		}
		if strings.Contains(line, "All:") && strings.Contains(line, "SSIM") == false {
			for _, field := range strings.Fields(line) {
				if strings.HasPrefix(field, "All:") {
					val := strings.TrimPrefix(field, "All:")
					if val != "inf" && val != "(inf)" {
						ssim, _ = strconv.ParseFloat(val, 64)
					} else {
						ssim = 1
					}
				}
			}
		}
		if strings.HasPrefix(strings.TrimSpace(line), "SSIM") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				ssim, _ = strconv.ParseFloat(strings.TrimSpace(strings.Split(parts[1], "(")[0]), 64)
			}
		}
	}
	return psnr, ssim, nil
}
