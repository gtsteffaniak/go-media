// Command gomedia-compare fetches seed media, generates ffmpeg fixtures, runs go-media vs ffprobe/ffmpeg checks, and writes a static HTML report.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	fs := flag.NewFlagSet("gomedia-compare", flag.ExitOnError)
	seedDir := fs.String("seed-dir", ".seed", "directory for pinned seed download")
	assetsDir := fs.String("assets", ".assets", "generated fixture output (gitignored)")
	reportDir := fs.String("report", "report_site", "static report site root")
	doSeed := fs.Bool("fetch-seed", true, "download/verify seed before generate")
	doGen := fs.Bool("generate", true, "regenerate asset matrix with ffmpeg")
	doRun := fs.Bool("run", true, "run comparisons and write report")
	expectationsPath := fs.String("expectations", "", "path to expectations.json (required with -gate)")
	gate := fs.Bool("gate", false, "exit non-zero when results differ from expectations")
	_ = fs.Parse(os.Args[1:])

	if err := runAll(*seedDir, *assetsDir, *reportDir, *doSeed, *doGen, *doRun, *expectationsPath, *gate); err != nil {
		fmt.Fprintf(os.Stderr, "compare: %v\n", err)
		os.Exit(1)
	}
}

// runAll orchestrates seed fetch, asset generation, comparison, and report output.
func runAll(seedDir, assetsDir, reportDir string, fetchSeed, generate, runChecks bool, expectationsPath string, gate bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// When invoked from repo root via script, paths are under test/compare.
	base := cwd
	if _, err := os.Stat(filepath.Join(base, "go.mod")); err != nil {
		base = filepath.Join(cwd, "test", "compare", "harness")
	}
	seedDir = absUnder(base, seedDir)
	assetsDir = absUnder(base, assetsDir)
	reportDir = absUnder(base, reportDir)

	var seedPath string
	if fetchSeed {
		seedPath, err = ensureSeed(seedDir)
		if err != nil {
			return err
		}
	} else {
		seedPath = filepath.Join(seedDir, seedFileName)
	}

	var assets []Asset
	if generate {
		assets, err = generateAssets(seedPath, assetsDir)
		if err != nil {
			return err
		}
	} else {
		assets, err = loadAssetManifest(assetsDir)
		if err != nil {
			return err
		}
	}

	if !runChecks {
		return nil
	}

	loadTimingGatePolicy(reportDir, expectationsPath)

	nested, err := runComparisons(assets, reportDir)
	if err != nil {
		return err
	}
	thumbDir := filepath.Join(reportDir, "media", "thumbs")
	_scalerBenchmarks = runScalerBenchmarks(assets, thumbDir)
	if err := writeReport(reportDir, assets, nested); err != nil {
		return err
	}
	flat := flattenResults(nested)
	full := FullReport{Summary: buildSummary(assets, nested), Assets: assets, Results: flat}
	printSummary(full)
	if gate {
		if expectationsPath == "" {
			return fmt.Errorf("-gate requires -expectations")
		}
		exp, err := loadExpectations(expectationsPath)
		if err != nil {
			return err
		}
		if err := validateGate(exp, full); err != nil {
			return err
		}
		fmt.Println("comparison gate: OK")
	}
	return nil
}

func loadTimingGatePolicy(reportDir, expectationsPath string) {
	path := expectationsPath
	if path == "" {
		path = filepath.Join(reportDir, "..", "expectations.json")
	}
	exp, err := loadExpectations(path)
	if err != nil {
		return
	}
	activeTimingGate = timingGatePolicyFromExpectations(exp)
}

func absUnder(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// loadAssetManifest rebuilds the asset list from a prior generate run.
func loadAssetManifest(assetsDir string) ([]Asset, error) {
	var ids = []struct {
		id, rel, cat, desc string
		sidecars           []string
	}{
		{"mp4_h264", "video/mp4_h264.mp4", "video", "MP4 H.264/AAC", nil},
		{"mp4_h264_baseline_1080", "video/mp4_h264_baseline_1080.mp4", "video", "H.264 Baseline 1080p", nil},
		{"mp4_h264_main_1080", "video/mp4_h264_main_1080.mp4", "video", "H.264 Main CABAC 1080p", nil},
		{"mp4_h264_high_1080", "video/mp4_h264_high_1080.mp4", "video", "H.264 High 8x8 1080p", nil},
		{"mov_h264", "video/mov_h264.mov", "video", "MOV H.264", nil},
		{"mkv_h264", "video/mkv_h264.mkv", "video", "MKV H.264", nil},
		{"mkv_vp8", "video/mkv_vp8.mkv", "video", "MKV VP8", nil},
		{"webm_vp8", "video/webm_vp8.webm", "video", "WebM VP8", nil},
		{"webm_vp9", "video/webm_vp9.webm", "video", "WebM VP9", nil},
		{"mjpeg_mp4", "video/mjpeg.mp4", "video", "MJPEG in MP4", nil},
		{"mp3_cbr", "audio/mp3_cbr.mp3", "audio", "MP3 CBR", nil},
		{"mp3_vbr", "audio/mp3_vbr.mp3", "audio", "MP3 VBR", nil},
		{"flac_tags", "audio/flac_tags.flac", "audio", "FLAC tags", []string{
			filepath.Join(assetsDir, "audio/flac_tags.lrc"),
		}},
		{"wav_pcm", "audio/wav_pcm.wav", "audio", "WAV", nil},
		{"ogg_vorbis", "audio/ogg_vorbis.ogg", "audio", "OGG Vorbis", nil},
		{"opus_ogg", "audio/opus.ogg", "audio", "Opus", nil},
		{"m4a_aac", "audio/m4a_aac.m4a", "audio", "M4A AAC", nil},
		{"mkv_subtitles", "video/mkv_subtitles.mkv", "video", "MKV subtitles", nil},
		{"mp4_sidecar_subs", "video/sample.mp4", "video", "MP4 sidecar subs", []string{
			filepath.Join(assetsDir, "video/sample.en.srt"),
			filepath.Join(assetsDir, "video/sample.en.vtt"),
			filepath.Join(assetsDir, "video/sample.en.ass"),
		}},
		{"mp4_embedded_subs", "video/mp4_embedded_subs.mp4", "video", "MP4 embedded subs", nil},
	}
	var assets []Asset
	for _, row := range ids {
		p := filepath.Join(assetsDir, row.rel)
		if _, err := os.Stat(p); err != nil {
			// Legacy sidecar fixture name before sample.mp4 rename.
			if row.id == "mp4_sidecar_subs" {
				legacy := filepath.Join(assetsDir, "video/sample_h264.mp4")
				if _, err2 := os.Stat(legacy); err2 == nil {
					p = legacy
				} else {
					continue
				}
			} else {
				continue
			}
		}
		assets = append(assets, Asset{ID: row.id, Path: p, Category: row.cat, Description: row.desc, Sidecars: row.sidecars})
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("no assets in %s (run with -generate)", assetsDir)
	}
	return assets, nil
}
