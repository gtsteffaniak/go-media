package main

import (
	"os"
	"sort"
	"time"

	"github.com/gtsteffaniak/go-media/pkg/media"
)

const thumbTimingWarmup = 11
const thumbTimingRuns = 15
const maxTimingRegressionPct = 0.50
const targetTimingRegressionPct = 0.10

type thumbTimingSample struct {
	GoMS     float64
	WriteMS  float64
	FFmpegMS float64
}

func benchmarkThumbnail(a Asset, opt media.ThumbnailOptions, preset thumbnailPresetConfig) thumbTimingSample {
	var goSamples, writeSamples []float64
	tmp := filepathJoinThumb(a.ID, preset.Name)
	for i := 0; i < thumbTimingWarmup+thumbTimingRuns; i++ {
		f, err := openAsset(a)
		if err != nil {
			continue
		}
		start := time.Now()
		tr, err := media.Thumbnail(f, f.Size(), opt)
		goMs := msSince(start)
		goSamples = append(goSamples, goMs)
		if err == nil {
			w0 := time.Now()
			_ = os.WriteFile(tmp, tr.Data, 0o644)
			writeSamples = append(writeSamples, goMs+msSince(w0))
		}
	}
	ffSamples := []float64{}
	ffPath := tmp + ".ff.jpg"
	for i := 0; i < thumbTimingWarmup+thumbTimingRuns; i++ {
		start := time.Now()
		kf := assetH264KeyframeOnly(a)
		_ = ffmpegThumbnailPreset(a, ffPath, thumbnailSeekSeconds(a.Path, 10, kf), preset, kf)
		ffSamples = append(ffSamples, msSince(start))
	}
	return thumbTimingSample{
		GoMS:     medianFloat(goSamples),
		WriteMS:  medianFloat(writeSamples),
		FFmpegMS: medianFloat(ffSamples),
	}
}

func medianFloat(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	sort.Float64s(v)
	return v[len(v)/2]
}

func filepathJoinThumb(assetID, preset string) string {
	return os.TempDir() + "/gomedia-thumb-" + assetID + "-" + preset + ".jpg"
}
