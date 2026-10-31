package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// FFProbeFormat holds ffprobe format fields used for comparison.
type FFProbeFormat struct {
	FormatName string
	Duration   float64
	Tags       map[string]string
}

// FFProbeStream is a simplified stream entry from ffprobe JSON.
type FFProbeStream struct {
	Index       int
	Tags        map[string]string
	CodecType   string
	CodecName   string
	Width       int
	Height      int
	SampleRate  string
	Channels    int
	AttachedPic int
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
		// All format and stream tags are needed for the metadata, lyrics and subtitle diff gate.
		"-show_entries", "format=format_name,duration:format_tags:stream=index,codec_type,codec_name,width,height,sample_rate,channels:stream_tags:stream_disposition=attached_pic",
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
			Index       int               `json:"index"`
			Tags        map[string]string `json:"tags"`
			CodecType   string            `json:"codec_type"`
			CodecName   string            `json:"codec_name"`
			Width       int               `json:"width"`
			Height      int               `json:"height"`
			SampleRate  string            `json:"sample_rate"`
			Channels    int               `json:"channels"`
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
	// Containers such as Ogg store Vorbis comments on the stream, so merge the
	// first value found for each key. Format tags win.
	for _, s := range raw.Streams {
		for k, v := range s.Tags {
			if _, ok := res.Format.Tags[k]; !ok {
				if res.Format.Tags == nil {
					res.Format.Tags = map[string]string{}
				}
				res.Format.Tags[k] = v
			}
		}
	}
	for _, s := range raw.Streams {
		res.Streams = append(res.Streams, FFProbeStream{
			Index: s.Index, Tags: s.Tags, CodecType: s.CodecType, CodecName: s.CodecName,
			Width: s.Width, Height: s.Height, SampleRate: s.SampleRate, Channels: s.Channels,
			AttachedPic: s.Disposition.AttachedPic,
		})
	}
	return res, nil
}

// ffprobeDuration returns duration in seconds (full probe; used for correctness checks).
func ffprobeDuration(path string) (float64, error) {
	p, err := probeFFprobe(path)
	if err != nil {
		return 0, err
	}
	return p.Format.Duration, nil
}

// ffprobeDurationHeader times ffprobe reading format duration only (matches lightweight header work).
func ffprobeDurationHeader(path string) (float64, error) {
	ffprobe, err := lookFFprobe()
	if err != nil {
		return 0, err
	}
	out, err := exec.Command(ffprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	).Output()
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, fmt.Errorf("ffprobe: empty duration")
	}
	return strconv.ParseFloat(s, 64)
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

// videoFrameSize returns the first video stream dimensions from ffprobe.
func videoFrameSize(path string) (width, height int, ok bool) {
	p, err := probeFFprobe(path)
	if err != nil {
		return 0, 0, false
	}
	for _, s := range p.Streams {
		if s.CodecType == "video" && s.Width > 0 && s.Height > 0 {
			return s.Width, s.Height, true
		}
	}
	return 0, 0, false
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
	if dur < 0.05 {
		return 0
	}
	return dur * seekPercent / 100.0
}

// ffmpegThumbnailPreset extracts one JPEG frame aligned with a media thumbnail preset.
// seekSec is decode time in seconds (use stss sync time for H.264 intra thumbnails; ctts is not applied).
// When keyframeOnly is true, ffmpeg decodes only IDR/I frames (matches go-media H.264 intra path).
func ffmpegThumbScaleFlags(preset thumbnailPresetConfig, keyframeOnly bool) string {
	// go-media H.264 intra thumbnails use area downscale in YCbCr (ThumbnailFilterArea).
	if keyframeOnly {
		return "flags=area"
	}
	return preset.FFScale
}

func ffmpegThumbnailPreset(a Asset, dest string, seekSec float64, preset thumbnailPresetConfig, keyframeOnly bool) error {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return err
	}
	inputPath, cleanup, err := ffmpegThumbInput(a.Path, a.ID)
	if err != nil {
		return err
	}
	defer cleanup()
	path := inputPath
	scaleFlags := ffmpegThumbScaleFlags(preset, keyframeOnly)
	vf := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:%s",
		preset.MaxWidth, preset.MaxHeight, scaleFlags)
	qv := ffmpegQScale(preset.Quality)
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if keyframeOnly {
		args = append(args, "-skip_frame", "nokey", "-skip_loop_filter", "all")
	}
	args = append(args, "-threads", "1")
	inputFirst := tsLikeContainer(a.Path)
	if seekSec > 0.001 && !inputFirst {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
	}
	args = append(args, "-i", path)
	if seekSec > 0.001 && inputFirst {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
	}
	args = append(args, "-frames:v", "1", "-vf", vf, "-q:v", fmt.Sprintf("%d", qv), dest)
	return exec.Command(ffmpeg, args...).Run()
}

func tsLikeContainer(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ts", ".m2ts", ".mts":
		return true
	default:
		return false
	}
}

// ffmpegRasterPNG writes a lossless PNG frame using the same seek/scale as thumbnails.
func ffmpegRasterPNG(a Asset, dest string, seekSec float64, preset thumbnailPresetConfig, keyframeOnly bool) error {
	ffmpeg, err := lookFFmpeg()
	if err != nil {
		return err
	}
	inputPath, cleanup, err := ffmpegThumbInput(a.Path, a.ID)
	if err != nil {
		return err
	}
	defer cleanup()
	path := inputPath
	scaleFlags := ffmpegThumbScaleFlags(preset, keyframeOnly)
	vf := fmt.Sprintf("scale=w='min(%d,iw)':h='min(%d,ih)':force_original_aspect_ratio=decrease:%s",
		preset.MaxWidth, preset.MaxHeight, scaleFlags)
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if keyframeOnly {
		args = append(args, "-skip_frame", "nokey")
	}
	// Same seek placement as ffmpegThumbnailPreset: TS fixtures are fed as
	// raw elementary streams, which need an output-side (decoding) seek.
	inputFirst := tsLikeContainer(a.Path)
	if seekSec > 0.001 && !inputFirst {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
	}
	args = append(args, "-i", path)
	if seekSec > 0.001 && inputFirst {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
	}
	args = append(args, "-frames:v", "1", "-vf", vf, dest)
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

// ffprobeKeyframeAtOrBefore returns the time (relative to the container
// start) of the last keyframe at or before sec, as ffprobe reports it. Assets
// whose go-media thumbnail decodes only intra pictures are compared against
// ffmpeg at that keyframe rather than at the exact seek time.
func ffprobeKeyframeAtOrBefore(path string, sec float64) (float64, bool) {
	ffprobe, err := lookFFprobe()
	if err != nil {
		return 0, false
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0",
		"-skip_frame", "nokey", "-show_entries", "frame=pts_time:format=start_time",
		"-read_intervals", fmt.Sprintf("%%+%.3f", sec+1),
		"-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, false
	}
	start := 0.0
	var times []float64
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSuffix(strings.TrimSpace(line), ",")
		v, err := strconv.ParseFloat(line, 64)
		if err != nil {
			continue
		}
		times = append(times, v)
	}
	if len(times) < 2 {
		return aviKeyframeAtOrBefore(ffprobe, path, sec)
	}
	// The format section (start_time) is printed after the frames.
	start, times = times[len(times)-1], times[:len(times)-1]
	best, ok := 0.0, false
	for _, t := range times {
		if rel := t - start; rel <= sec+1e-6 {
			best, ok = rel, true
		}
	}
	return best, ok
}

// assetIntraSeekOnly reports assets whose go-media thumbnail decodes the
// keyframe at or before the seek point instead of the exact frame.
func assetIntraSeekOnly(id string) bool {
	switch id {
	case "mpegts_mpeg2", "mpegts_h264", "avi_h264", "avi_mpeg4", "asf_wmv3",
		"webm_vp8", "mkv_vp8", "webm_vp9":
		return true
	}
	return false
}

// aviKeyframeAtOrBefore handles AVI, where ffprobe reports no frame
// timestamps: keyframe time is packet index / frame rate.
func aviKeyframeAtOrBefore(ffprobe, path string, sec float64) (float64, bool) {
	if strings.ToLower(filepath.Ext(path)) != ".avi" {
		return 0, false
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate:packet=flags", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return 0, false
	}
	var num, den float64
	if _, err := fmt.Sscanf(strings.TrimSuffix(lines[len(lines)-1], ","), "%f/%f", &num, &den); err != nil || num <= 0 || den <= 0 {
		return 0, false
	}
	fps := num / den
	best, ok := 0.0, false
	for i, l := range lines[:len(lines)-1] {
		t := float64(i) / fps
		if t > sec+1e-6 {
			break
		}
		if strings.HasPrefix(l, "K") {
			best, ok = t, true
		}
	}
	return best, ok
}
