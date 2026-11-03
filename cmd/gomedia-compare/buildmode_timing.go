package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// BuildMode names stacked report tiers (PGO/SIMD are incremental on the default CGO_ENABLED=0 build).
type BuildMode string

const (
	// Tier 1: portable Go only (no asm kernels, no PGO, no simd experiment).
	BuildModePurego BuildMode = "purego"
	// Tier 2 (production): tier 1 + -pgo=default.pgo — support gate and recommended ship build.
	BuildModePuregoPGO BuildMode = "purego_pgo"
	// Tier 3: tier 2 + GOEXPERIMENT=simd (and simd kernel paths where implemented).
	BuildModePuregoPGOSIMD BuildMode = "purego_pgo_simd"
)

// ShippedTimingGateBuild documents which binary the in-process support gate uses (see expectations.json timingGate).
const ShippedTimingGateBuild = "purego_pgo_in_process"

// BuildModeTimingRow is one asset/preset timed in a specific build mode vs ffmpeg.
type BuildModeTimingRow struct {
	AssetID        string    `json:"assetId"`
	Check          string    `json:"check"`
	ThumbPreset    string    `json:"thumbPreset"`
	BuildMode      BuildMode `json:"buildMode"`
	Tier           int       `json:"tier"` // 1..3 stacked tiers
	GoMediaWrite   float64   `json:"goMediaWriteMs"`
	FFmpegMS       float64   `json:"ffmpegMs"`
	DeltaPct       float64   `json:"timingDeltaPct"`
	GatePass       bool      `json:"timingGatePass"`       // vs ffmpeg cap when GateApplicable
	GateApplicable bool      `json:"timingGateApplicable"` // false for experiment tier 3 (GOEXPERIMENT=simd)
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
	tierOf := map[BuildMode]int{
		BuildModePurego:        1,
		BuildModePuregoPGO:     2,
		BuildModePuregoPGOSIMD: 3,
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
				ms, err := runThumbModeBench(bin, a.Path, preset.Name, mode)
				if err != nil {
					fmt.Fprintf(os.Stderr, "thumbmode %s %s %s: %v\n", mode, a.ID, preset.Name, err)
					continue
				}
				delta := (ms - ffMs) / ffMs
				gateApplicable := mode == BuildModePuregoPGO
				gatePass := !gateApplicable || delta <= limit
				out = append(out, BuildModeTimingRow{
					AssetID: a.ID, Check: string(check), ThumbPreset: preset.Name,
					BuildMode: mode, Tier: tierOf[mode],
					GoMediaWrite: ms, FFmpegMS: ffMs, DeltaPct: delta,
					GatePass: gatePass, GateApplicable: gateApplicable,
				})
			}
		}
	}
	return out
}

func buildThumbModeBinaries(repoRoot string) (map[BuildMode]string, error) {
	dir := filepath.Join(os.TempDir(), "gomedia-thumbmode-bins")
	_ = os.MkdirAll(dir, 0o755)
	pgo := filepath.Join(repoRoot, "default.pgo")
	if _, err := os.Stat(pgo); err != nil {
		return nil, fmt.Errorf("default.pgo missing (run make pgo): %w", err)
	}
	out := map[BuildMode]string{
		BuildModePurego:        filepath.Join(dir, "thumbmode-purego"),
		BuildModePuregoPGO:     filepath.Join(dir, "thumbmode-purego-pgo"),
		BuildModePuregoPGOSIMD: filepath.Join(dir, "thumbmode-purego-pgo-simd"),
	}
	baseEnv := append(os.Environ(), "CGO_ENABLED=0")
	build := func(path string, usePGO bool, goexp string) error {
		args := []string{"build", "-o", path}
		if usePGO {
			args = append(args, "-pgo="+pgo)
		}
		args = append(args, "./cmd/gomedia-thumbmode")
		cmd := exec.Command("go", args...)
		cmd.Dir = repoRoot
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
	if err := build(out[BuildModePurego], false, ""); err != nil {
		return nil, err
	}
	if err := build(out[BuildModePuregoPGO], true, ""); err != nil {
		return nil, err
	}
	if err := build(out[BuildModePuregoPGOSIMD], true, "simd"); err != nil {
		return nil, err
	}
	return out, nil
}

func runThumbModeBench(bin, assetPath, preset string, mode BuildMode) (float64, error) {
	// -measure-gated lets gated profiles (H.264 Baseline) decode so every tier times them too.
	cmd := exec.Command(bin, "-asset", assetPath, "-preset", preset, "-seek", "10", "-measure-gated")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if mode == BuildModePuregoPGOSIMD {
		cmd.Env = append(cmd.Env, "GOEXPERIMENT=simd")
	}
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}
