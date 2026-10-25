package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// BuildMode names a go-media binary variant for timing comparisons.
type BuildMode string

const (
	BuildModeDefault BuildMode = "default"
	BuildModePurego  BuildMode = "purego"
	BuildModeSIMD    BuildMode = "simd"
)

// BuildModeTimingRow is one asset/preset timed in a specific build mode vs ffmpeg.
type BuildModeTimingRow struct {
	AssetID      string    `json:"assetId"`
	Check        string    `json:"check"`
	ThumbPreset  string    `json:"thumbPreset"`
	BuildMode    BuildMode `json:"buildMode"`
	GoMediaWrite float64   `json:"goMediaWriteMs"`
	FFmpegMS     float64   `json:"ffmpegMs"`
	DeltaPct     float64   `json:"timingDeltaPct"`
	GatePass     bool      `json:"timingGatePass"`
}

var _buildModeTimings []BuildModeTimingRow

func findRepoRoot(start string) string {
	dir := start
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "go.mod")
		if b, err := os.ReadFile(p); err == nil {
			s := string(b)
			if strings.HasPrefix(s, "module github.com/gtsteffaniak/go-media\n") {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	return start
}

func collectBuildModeTimings(assets []Asset, flat []CheckResult, repoRoot string) []BuildModeTimingRow {
	if runtime.GOARCH != "amd64" {
		return nil
	}
	bins, err := buildThumbModeBinaries(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "build mode binaries: %v (skipping mode timings)\n", err)
		return nil
	}
	limit := activeTimingGate.maxRegression()
	var out []BuildModeTimingRow
	ffByKey := map[string]float64{}
	for _, r := range flat {
		if r.Check != CheckThumbnailSmall && r.Check != CheckThumbnailLarge {
			continue
		}
		if r.FFmpegMS <= 0 {
			continue
		}
		ffByKey[r.AssetID+"|"+r.ThumbPreset] = r.FFmpegMS
	}
	for _, a := range assets {
		if a.Category != "video" {
			continue
		}
		for _, preset := range thumbnailPresets {
			ffMs := ffByKey[a.ID+"|"+preset.Name]
			if ffMs <= 0 {
				continue
			}
			check := thumbnailCheckName(preset)
			for mode, bin := range bins {
				ms, err := runThumbModeBench(bin, a.Path, preset.Name)
				if err != nil {
					fmt.Fprintf(os.Stderr, "thumbmode %s %s %s: %v\n", mode, a.ID, preset.Name, err)
					continue
				}
				delta := (ms - ffMs) / ffMs
				out = append(out, BuildModeTimingRow{
					AssetID: a.ID, Check: string(check), ThumbPreset: preset.Name,
					BuildMode: mode, GoMediaWrite: ms, FFmpegMS: ffMs, DeltaPct: delta,
					GatePass: delta <= limit,
				})
			}
		}
	}
	return out
}

func buildThumbModeBinaries(repoRoot string) (map[BuildMode]string, error) {
	dir := filepath.Join(os.TempDir(), "gomedia-thumbmode-bins")
	_ = os.MkdirAll(dir, 0o755)
	harnessDir := filepath.Join(repoRoot, "test/compare/harness")
	out := map[BuildMode]string{
		BuildModeDefault: filepath.Join(dir, "thumbmode-default"),
		BuildModePurego:  filepath.Join(dir, "thumbmode-purego"),
		BuildModeSIMD:    filepath.Join(dir, "thumbmode-simd"),
	}
	pgo := filepath.Join(repoRoot, "default.pgo")
	baseEnv := append(os.Environ(), "CGO_ENABLED=0")
	build := func(path string, tags string, goexp string) error {
		args := []string{"build", "-o", path}
		if tags != "" {
			args = append(args, "-tags", tags)
		}
		if _, err := os.Stat(pgo); err == nil {
			args = append(args, "-pgo="+pgo)
		}
		args = append(args, "./thumbmode")
		cmd := exec.Command("go", args...)
		cmd.Dir = harnessDir
		env := baseEnv
		if goexp != "" {
			env = append(env, "GOEXPERIMENT="+goexp)
		}
		cmd.Env = env
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%v: %s", err, stderr.String())
		}
		return nil
	}
	if err := build(out[BuildModeDefault], "", ""); err != nil {
		return nil, err
	}
	if err := build(out[BuildModePurego], "purego", ""); err != nil {
		return nil, err
	}
	if err := build(out[BuildModeSIMD], "", "simd"); err != nil {
		return nil, err
	}
	return out, nil
}

func runThumbModeBench(bin, assetPath, preset string) (float64, error) {
	cmd := exec.Command(bin, "-asset", assetPath, "-preset", preset, "-seek", "10")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if strings.Contains(bin, "simd") {
		cmd.Env = append(cmd.Env, "GOEXPERIMENT=simd")
	}
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}
