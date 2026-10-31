package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gtsteffaniak/go-media/pkg/media"
	"github.com/gtsteffaniak/go-media/internal/h264"
)

var activeTimingGate = defaultTimingGatePolicy()

// runComparisons executes all checks for every asset and writes thumbnail media under reportDir.
func runComparisons(assets []Asset, reportDir string) ([][]CheckResult, error) {
	thumbDir := filepath.Join(reportDir, "media", "thumbs")
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return nil, err
	}
	var results [][]CheckResult
	for _, a := range assets {
		results = append(results, compareAsset(a, thumbDir))
	}
	return results, nil
}

// compareAsset runs the full check matrix for one asset.
func compareAsset(a Asset, thumbDir string) []CheckResult {
	var out []CheckResult
	out = append(out, checkDuration(a))
	out = append(out, checkProbe(a))
	out = append(out, checkTags(a))
	out = append(out, checkArtwork(a))
	out = append(out, checkLyrics(a))
	out = append(out, checkSubtitles(a))
	for _, preset := range thumbnailPresets {
		out = append(out, checkThumbnailPreset(a, thumbDir, preset))
	}
	return out
}

func openAsset(a Asset) (*fileAt, error) {
	return openFileAt(a.Path)
}

// checkDuration compares media.Duration with ffprobe.
func checkDuration(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckDuration}
	f, size, err := openAssetOnDisk(a.Path)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	defer f.Close()
	start := time.Now()
	dr, gErr := media.Duration(f, size)
	res.GoMediaMS = msSince(start)

	ffStart := time.Now()
	_, _ = ffprobeDurationHeader(a.Path) // timed: header-only ffprobe (not full stream probe)
	res.FFmpegMS = msSince(ffStart)

	if gErr != nil {
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		return res
	}
	want, fErr := ffprobeDuration(a.Path)
	if fErr != nil {
		res.Status = StatusError
		res.Detail = fErr.Error()
		return res
	}
	got := dr.Duration.Seconds()
	res.DurationDelta = got - want
	if math.Abs(res.DurationDelta) > 0.35 {
		res.Status = StatusMismatch
		res.Detail = fmt.Sprintf("go=%.3fs ffprobe=%.3fs src=%s", got, want, dr.Source)
	} else {
		res.Status = StatusSupported
		res.Detail = fmt.Sprintf("delta=%.3fs src=%s", res.DurationDelta, dr.Source)
	}
	return res
}

// checkProbe compares high-level probe fields with ffprobe.
func checkProbe(a Asset) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: CheckProbe}
	f, size, err := openAssetOnDisk(a.Path)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	defer f.Close()
	start := time.Now()
	info, gErr := media.Probe(f, size)
	res.GoMediaMS = msSince(start)

	ffStart := time.Now()
	ff, fErr := probeFFprobe(a.Path)
	res.FFmpegMS = msSince(ffStart)

	if gErr != nil {
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		return res
	}
	if fErr != nil {
		res.Status = StatusError
		res.Detail = fErr.Error()
		return res
	}
	var diffs []string
	if ff.Format.Duration > 0 {
		delta := info.Duration.Seconds() - ff.Format.Duration
		if math.Abs(delta) > 0.35 {
			diffs = append(diffs, fmt.Sprintf("duration: go=%.3f ff=%.3f", info.Duration.Seconds(), ff.Format.Duration))
		}
	}
	vidGo, audGo := streamCounts(info.Streams)
	vidFF, audFF := ffStreamCounts(ff.Streams, true)
	if vidGo != vidFF {
		diffs = append(diffs, fmt.Sprintf("video streams: go=%d ff=%d", vidGo, vidFF))
	}
	if audGo != audFF {
		diffs = append(diffs, fmt.Sprintf("audio streams: go=%d ff=%d", audGo, audFF))
	}
	res.FieldDiffs = diffs
	if len(diffs) > 0 {
		res.Status = StatusMismatch
		res.Detail = strings.Join(diffs, "; ")
	} else {
		res.Status = StatusSupported
		res.Detail = fmt.Sprintf("format=%s streams=%d", info.FormatName, len(info.Streams))
	}
	return res
}

func thumbnailCheckName(preset thumbnailPresetConfig) CheckName {
	if preset.Name == "large" {
		return CheckThumbnailLarge
	}
	return CheckThumbnailSmall
}

func mediaPreset(preset thumbnailPresetConfig) media.ThumbnailOptions {
	opt := media.OptionsForPreset(media.ThumbnailPreset(preset.Name), 10)
	return opt
}

// checkThumbnailPreset compares go-media vs ffmpeg for one thumbnail preset.
//
// A typed_unsupported result is never asserted. When media rejects a profile (so the
// library returned ErrUnsupportedProfile), the check is repeated with the profile
// restriction lifted, so the row carries real numbers and a reason:
//   - "functionality:" the decoder still cannot handle it, or the output fails the quality gate;
//   - "performance:" it decodes correctly but misses the ffmpeg timing gate (see expectations.json);
//   - if it meets the gate, the row becomes a mismatch telling the developer to enable it.
func checkThumbnailPreset(a Asset, thumbDir string, preset thumbnailPresetConfig) CheckResult {
	res := checkThumbnailPresetOnce(a, thumbDir, preset)
	if res.Status != StatusTypedUnsupported {
		return res
	}
	if !errors.Is(res.goErr, media.ErrUnsupportedProfile) {
		// Not a profile restriction (e.g. a codec or container go-media does not implement).
		res.Detail = "functionality: " + res.Detail
		return res
	}
	first := res
	h264.AllowGatedProfiles(true)
	defer h264.AllowGatedProfiles(false)
	res = checkThumbnailPresetOnce(a, thumbDir, preset)
	switch {
	case res.goErr != nil && errors.Is(res.goErr, media.ErrUnsupportedProfile):
		first.Detail = "functionality: " + first.Detail
		return first
	case res.goErr != nil:
		first.Detail = fmt.Sprintf("functionality: decode fails with the restriction lifted: %v", res.goErr)
		return first
	case res.TimingGateFail:
		res.Status = StatusTypedUnsupported
		res.Detail = "performance: decodes correctly but misses the ffmpeg gate (measured with the profile restriction lifted); " + res.Detail
	case res.Status == StatusSupported:
		res.Status = StatusMismatch
		res.Detail = "meets the ffmpeg timing gate with the profile restriction lifted: enable it (see internal/h264/sps.go) and update expectations.json; " + res.Detail
	case res.Status == StatusMismatch:
		res.Status = StatusTypedUnsupported
		res.Detail = "functionality: output fails the quality gate with the restriction lifted; " + res.Detail
	}
	return res
}

func checkThumbnailPresetOnce(a Asset, thumbDir string, preset thumbnailPresetConfig) CheckResult {
	res := CheckResult{AssetID: a.ID, Check: thumbnailCheckName(preset), ThumbPreset: preset.Name}
	if a.Category != "video" {
		res.Status = StatusSkipped
		res.Detail = "audio fixture"
		return res
	}
	f, err := openAsset(a)
	if err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		return res
	}
	kfOnly := assetH264KeyframeOnly(a)
	seekSec := thumbnailSeekSeconds(a.Path, 10, kfOnly)
	opt := compareThumbnailOptions(a.Path, preset, kfOnly)

	tr, gErr := media.Thumbnail(f, f.Size(), opt)

	base := a.ID + "_" + preset.Name
	goPath := filepath.Join(thumbDir, base+"_go.jpg")
	ffPath := filepath.Join(thumbDir, base+"_ff.jpg")
	goPNG := filepath.Join(thumbDir, base+"_go.png")
	ffPNG := filepath.Join(thumbDir, base+"_ff.png")

	ffErr := ffmpegThumbnailPreset(a, ffPath, seekSec, preset, kfOnly)

	if gErr != nil {
		res.goErr = gErr
		res.Status = classifyGoErr(gErr)
		res.Detail = gErr.Error()
		if ffErr == nil {
			res.ThumbFFPath = relReportPath(ffPath)
		}
		return res
	}
	if err := os.WriteFile(goPath, tr.Data, 0o644); err != nil {
		res.Status = StatusError
		res.Detail = err.Error()
		if ffErr == nil {
			res.ThumbFFPath = relReportPath(ffPath)
		}
		return res
	}
	if ffErr != nil {
		res.Status = StatusError
		res.Detail = ffErr.Error()
		res.ThumbGoPath = relReportPath(goPath)
		return res
	}
	res.ThumbGoPath = relReportPath(goPath)
	res.ThumbFFPath = relReportPath(ffPath)

	if kfOnly {
		if dPSNR, dSSIM, err := h264DecodeYUVMetrics(a.Path, 10); err == nil {
			res.PSNRDecode = dPSNR
			res.SSIMDecode = dSSIM
		}
	} else {
		_ = os.Remove(goPNG)
		decOpt := opt
		decOpt.Preset = ""
		decOpt.Fast = false
		rasterPreset := preset
		if w, h, ok := videoFrameSize(a.Path); ok {
			decOpt.MaxWidth = w
			decOpt.MaxHeight = h
			rasterPreset.MaxWidth = w
			rasterPreset.MaxHeight = h
		}
		f2, err := openAsset(a)
		if err == nil {
			if goRaw, err := media.ThumbnailRasterPNG(f2, f2.Size(), decOpt); err == nil {
				if err := os.WriteFile(goPNG, goRaw, 0o644); err == nil {
					if err := ffmpegRasterPNG(a, ffPNG, seekSec, rasterPreset, false); err == nil {
						if dPSNR, dSSIM, err := imagePSNRSSIM(ffPNG, goPNG); err == nil {
							res.PSNRDecode = dPSNR
							res.SSIMDecode = dSSIM
						}
					}
				}
			}
		}
	}

	psnr, ssim, mErr := imagePSNRSSIM(ffPath, goPath)
	res.PSNR = psnr
	res.SSIM = ssim
	if mErr != nil {
		res.Detail = mErr.Error()
	}
	res.Detail = fmt.Sprintf("jpeg PSNR=%.1f SSIM=%.3f decode PSNR=%.1f SSIM=%.3f",
		res.PSNR, res.SSIM, res.PSNRDecode, res.SSIMDecode)

	timing := benchmarkThumbnail(a, opt, preset)
	res.GoMediaMS = timing.GoMS
	res.GoMediaWriteMS = timing.WriteMS
	res.FFmpegMS = timing.FFmpegMS
	if res.FFmpegMS > 0 {
		goCmp := res.GoMediaWriteMS
		if goCmp <= 0 {
			goCmp = res.GoMediaMS
		}
		delta := (goCmp - res.FFmpegMS) / res.FFmpegMS
		res.TimingDeltaPct = &delta
		// Allow 1pp slack on the timing gate (median-of-15 variance on cloud CI).
		if delta > activeTimingGate.maxRegression()+0.01 && activeTimingGate.enforced(res.Check) {
			res.TimingGateFail = true
		}
	}

	if res.TimingGateFail {
		res.Status = StatusMismatch
		res.Detail += fmt.Sprintf("; timing gate: write %.0fms vs ffmpeg %.0fms (%.0f%% > %.0f%%)",
			res.GoMediaWriteMS, res.FFmpegMS, *res.TimingDeltaPct*100, activeTimingGate.maxRegression()*100)
		return res
	}
	if res.TimingDeltaPct != nil && *res.TimingDeltaPct > activeTimingGate.TargetMaxRegressionFraction {
		res.Detail += fmt.Sprintf("; timing note: %.0f%% over ffmpeg target %.0f%%",
			*res.TimingDeltaPct*100, activeTimingGate.TargetMaxRegressionFraction*100)
	}
	if kfOnly && res.PSNRDecode > 0 && !h264DecodeQualityPass(res.PSNRDecode, res.SSIMDecode) {
		res.Status = StatusMismatch
		res.Detail += fmt.Sprintf("; decode gate: PSNR %.1f SSIM %.3f (need PSNR≥45 or SSIM≥0.98)",
			res.PSNRDecode, res.SSIMDecode)
		return res
	}
	if res.SSIM >= 0.85 || res.PSNR >= 25 {
		res.Status = StatusSupported
	} else if tr.Width > 0 {
		res.Status = StatusMismatch
	} else {
		res.Status = StatusSupported
	}
	return res
}

// h264DecodeQualityPass accepts high structural SSIM even when YUV PSNR is modest (matrix/level drift vs libavcodec).
func h264DecodeQualityPass(psnr, ssim float64) bool {
	if psnr >= 45 {
		return true
	}
	if ssim >= 0.98 {
		return true
	}
	return false
}

func assetH264KeyframeOnly(a Asset) bool {
	if a.Category != "video" {
		return false
	}
	switch a.ID {
	case "mpegts_h264", "mpegts_mpeg2", "avi_h264", "avi_mjpeg", "avi_mpeg4",
		"webm_vp8", "webm_vp9", "mkv_vp8", "asf_wmv3":
		return false
	}
	if strings.Contains(a.ID, "h264") || strings.HasPrefix(a.ID, "mp4_h264_") {
		return true
	}
	switch a.ID {
	case "mp4_sidecar_subs", "mp4_embedded_subs":
		return true
	}
	return false
}

func classifyGoErr(err error) CapabilityStatus {
	if errors.Is(err, media.ErrUnsupported) || errors.Is(err, media.ErrUnsupportedProfile) {
		return StatusTypedUnsupported
	}
	return StatusError
}

func streamCounts(streams []media.Stream) (video, audio int) {
	for _, s := range streams {
		switch s.Type {
		case media.StreamVideo:
			video++
		case media.StreamAudio:
			audio++
		}
	}
	return video, audio
}

func ffStreamCounts(streams []FFProbeStream, skipAttachedPic bool) (video, audio int) {
	for _, s := range streams {
		if skipAttachedPic && s.AttachedPic == 1 {
			continue
		}
		switch s.CodecType {
		case "video":
			video++
		case "audio":
			audio++
		}
	}
	return video, audio
}

func msSince(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func relReportPath(abs string) string {
	// Stored relative to report_site root for static hosting.
	const marker = "report_site/"
	if i := strings.Index(abs, marker); i >= 0 {
		return abs[i+len(marker):]
	}
	return filepath.Base(abs)
}
