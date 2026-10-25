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

// BuildMode names stacked report tiers (all use -tags purego; PGO/SIMD are incremental).
type BuildMode string

const (
	// Tier 1: portable Go only (no asm kernels, no PGO, no simd experiment).
	BuildModePurego BuildMode = "purego"
	// Tier 2: tier 1 + -pgo=default.pgo.
	BuildModePuregoPGO BuildMode = "purego_pgo"
	// Tier 3: tier 2 + GOEXPERIMENT=simd (and simd kernel paths where implemented).
	BuildModePuregoPGOSIMD BuildMode = "purego_pgo_simd"
)

// ShippedTimingGateBuild documents which binary the ≤20% support gate uses (in-process harness).
const ShippedTimingGateBuild = "amd64_asm_kernels_pgo_in_process"

// BuildModeTimingRow is one asset/preset timed in a specific build mode vs ffmpeg.
type BuildModeTimingRow struct {
	AssetID      string    `json:"assetId"`
	Check        string    `json:"check"`
	ThumbPreset  string    `json:"thumbPreset"`
	BuildMode    BuildMode `json:"buildMode"`
	Tier         int       `json:"tier"` // 1..3 stacked tiers
	GoMediaWrite float64   `json:"goMediaWriteMs"`
	FFmpegMS     float64   `json:"ffmpegMs"`
	DeltaPct     float64   `json:"timingDeltaPct"`
	GatePass     bool      `json:"timingGatePass"` // vs ffmpeg ≤20%; informational for tiers
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
	tierOf := map[BuildMode]int{
		BuildModePurego:         1,
		BuildModePuregoPGO:      2,
		BuildModePuregoPGOSIMD:  3,
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
				out = append(out, BuildModeTimingRow{
					AssetID: a.ID, Check: string(check), ThumbPreset: preset.Name,
					BuildMode: mode, Tier: tierOf[mode],
					GoMediaWrite: ms, FFmpegMS: ffMs, DeltaPct: delta,
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
		args := []string{"build", "-o", path, "-tags", "purego"}
		if usePGO {
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
	cmd := exec.Command(bin, "-asset", assetPath, "-preset", preset, "-seek", "10")
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
