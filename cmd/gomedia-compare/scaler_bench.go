package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

// ScalerBenchRow is one scaler × preset × asset measurement.
type ScalerBenchRow struct {
	AssetID   string  `json:"assetId"`
	Preset    string  `json:"preset"`
	Scaler    string  `json:"scaler"`
	GoMediaMs float64 `json:"goMediaMs"`
	GoWriteMs float64 `json:"goWriteMs"`
	FFmpegMs  float64 `json:"ffmpegMs"`
	DeltaPct  float64 `json:"deltaPct"`
	PSNR      float64 `json:"psnr"`
	SSIM      float64 `json:"ssim"`
}

const timingWarmup = 2
const timingRuns = 7

func runScalerBenchmarks(assets []Asset, thumbDir string) []ScalerBenchRow {
	var out []ScalerBenchRow
	benchAssets := []string{"webm_vp8", "mkv_vp8", "mjpeg_mp4"}
	for _, a := range assets {
		if !containsStr(benchAssets, a.ID) {
			continue
		}
		for _, preset := range thumbnailPresets {
			ffMs := medianFFmpegThumb(a, preset)
			for _, filter := range media.AllThumbnailFilters() {
				out = append(out, benchScalerRow(a, preset, filter, thumbDir, ffMs))
			}
		}
	}
	return out
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func medianFFmpegThumb(a Asset, preset thumbnailPresetConfig) float64 {
	var samples []float64
	ffPath := filepath.Join(os.TempDir(), "gomedia-ff-bench.jpg")
	for i := 0; i < timingWarmup+timingRuns; i++ {
		start := time.Now()
		kf := assetH264KeyframeOnly(a)
		_ = ffmpegThumbnailPreset(a.Path, ffPath, thumbnailSeekSeconds(a.Path, 10, kf), preset, kf)
		samples = append(samples, msSince(start))
	}
	sort.Float64s(samples)
	return samples[len(samples)/2]
}

func benchScalerRow(a Asset, preset thumbnailPresetConfig, filter media.ThumbnailFilter, thumbDir string, ffMs float64) ScalerBenchRow {
	opt := mediaPreset(preset)
	opt.Filter = filter
	var goSamples, writeSamples []float64
	outPath := filepath.Join(thumbDir, fmt.Sprintf("_bench_%s_%s_%s.jpg", a.ID, preset.Name, media.ThumbnailFilterLabel(filter)))
	for i := 0; i < timingWarmup+timingRuns; i++ {
		f2, err := openAsset(a)
		if err != nil {
			break
		}
		start := time.Now()
		tr, err := media.Thumbnail(f2, f2.Size(), opt)
		goSamples = append(goSamples, msSince(start))
		if err == nil {
			wStart := time.Now()
			_ = os.WriteFile(outPath, tr.Data, 0o644)
			writeSamples = append(writeSamples, msSince(start)+msSince(wStart))
		}
	}
	sort.Float64s(goSamples)
	sort.Float64s(writeSamples)
	goMed := median(goSamples)
	writeMed := median(writeSamples)
	delta := 0.0
	if ffMs > 0 {
		delta = (writeMed - ffMs) / ffMs
	}
	psnr, ssim := scalerQuality(a, preset, filter, thumbDir)
	return ScalerBenchRow{
		AssetID: a.ID, Preset: preset.Name, Scaler: media.ThumbnailFilterLabel(filter),
		GoMediaMs: goMed, GoWriteMs: writeMed, FFmpegMs: ffMs, DeltaPct: delta,
		PSNR: psnr, SSIM: ssim,
	}
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

func scalerQuality(a Asset, preset thumbnailPresetConfig, filter media.ThumbnailFilter, thumbDir string) (float64, float64) {
	opt := mediaPreset(preset)
	opt.Filter = filter
	f, err := openAsset(a)
	if err != nil {
		return 0, 0
	}
	tr, err := media.Thumbnail(f, f.Size(), opt)
	if err != nil {
		return 0, 0
	}
	base := fmt.Sprintf("_scaler_%s_%s_%s", a.ID, preset.Name, media.ThumbnailFilterLabel(filter))
	goPath := filepath.Join(thumbDir, base+"_go.jpg")
	ffPath := filepath.Join(thumbDir, base+"_ff.jpg")
	_ = os.WriteFile(goPath, tr.Data, 0o644)
	kf := assetH264KeyframeOnly(a)
	_ = ffmpegThumbnailPreset(a.Path, ffPath, thumbnailSeekSeconds(a.Path, 10, kf), preset, kf)
	psnr, ssim, err := imagePSNRSSIM(ffPath, goPath)
	if err != nil {
		return 0, 0
	}
	return psnr, ssim
}
