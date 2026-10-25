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
	_ = fs.Parse(os.Args[1:])

	if err := runAll(*seedDir, *assetsDir, *reportDir, *doSeed, *doGen, *doRun); err != nil {
		fmt.Fprintf(os.Stderr, "compare: %v\n", err)
		os.Exit(1)
	}
}

// runAll orchestrates seed fetch, asset generation, comparison, and report output.
func runAll(seedDir, assetsDir, reportDir string, fetchSeed, generate, runChecks bool) error {
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

	nested, err := runComparisons(assets, reportDir)
	if err != nil {
		return err
	}
	if err := writeReport(reportDir, assets, nested); err != nil {
		return err
	}
	flat := flattenResults(nested)
	printSummary(FullReport{Summary: buildSummary(assets, nested), Assets: assets, Results: flat})
	return nil
}

func absUnder(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// loadAssetManifest rebuilds the asset list from a prior generate run.
func loadAssetManifest(assetsDir string) ([]Asset, error) {
	// Minimal rediscovery: walk known layout from generateAssets.
	var ids = []struct {
		id, rel, cat, desc string
	}{
		{"mp4_h264", "video/mp4_h264.mp4", "video", "MP4 H.264/AAC"},
		{"mov_h264", "video/mov_h264.mov", "video", "MOV H.264"},
		{"mkv_h264", "video/mkv_h264.mkv", "video", "MKV H.264"},
		{"webm_vp8", "video/webm_vp8.webm", "video", "WebM VP8"},
		{"webm_vp9", "video/webm_vp9.webm", "video", "WebM VP9"},
		{"mjpeg_mp4", "video/mjpeg.mp4", "video", "MJPEG in MP4"},
		{"mp3_cbr", "audio/mp3_cbr.mp3", "audio", "MP3 CBR"},
		{"mp3_vbr", "audio/mp3_vbr.mp3", "audio", "MP3 VBR"},
		{"flac_tags", "audio/flac_tags.flac", "audio", "FLAC tags"},
		{"wav_pcm", "audio/wav_pcm.wav", "audio", "WAV"},
		{"ogg_vorbis", "audio/ogg_vorbis.ogg", "audio", "OGG Vorbis"},
		{"opus_ogg", "audio/opus.ogg", "audio", "Opus"},
		{"m4a_aac", "audio/m4a_aac.m4a", "audio", "M4A AAC"},
		{"mkv_subtitles", "video/mkv_subtitles.mkv", "video", "MKV subtitles"},
		{"mp4_sidecar_subs", "video/sample_h264.mp4", "video", "MP4 sidecar subs"},
	}
	var assets []Asset
	for _, row := range ids {
		p := filepath.Join(assetsDir, row.rel)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		assets = append(assets, Asset{ID: row.id, Path: p, Category: row.cat, Description: row.desc})
	}
	if len(assets) == 0 {
		return nil, fmt.Errorf("no assets in %s (run with -generate)", assetsDir)
	}
	return assets, nil
}
