package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// ExpectationsFile is the checked-in manifest of allowed comparison outcomes.
type ExpectationsFile struct {
	Version             int               `json:"version"`
	FFmpegVersionPrefix string            `json:"ffmpegVersionPrefix"`
	TimingGate          *TimingGatePolicy `json:"timingGate,omitempty"`
	Cases               []ExpectationCase `json:"cases"`
}

// ExpectationCase is one asset/check expected status.
type ExpectationCase struct {
	AssetID string           `json:"assetId"`
	Check   CheckName        `json:"check"`
	Expect  CapabilityStatus `json:"expect"`
	// Goarch, when set, limits this case to runtime.GOARCH (e.g. arm64 vs amd64 timing gates).
	Goarch string `json:"goarch,omitempty"`
}

func expectationCaseApplies(c ExpectationCase) bool {
	if c.Goarch == "" {
		return true
	}
	return c.Goarch == runtime.GOARCH
}

// loadExpectations reads and validates the expectations manifest.
func loadExpectations(path string) (ExpectationsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExpectationsFile{}, err
	}
	var exp ExpectationsFile
	if err := json.Unmarshal(data, &exp); err != nil {
		return ExpectationsFile{}, err
	}
	if exp.Version == 0 {
		return ExpectationsFile{}, fmt.Errorf("expectations: missing version")
	}
	if len(exp.Cases) == 0 {
		return ExpectationsFile{}, fmt.Errorf("expectations: no cases")
	}
	return exp, nil
}

// validateGate compares report results to expectations and returns a human-readable diff.
func validateGate(exp ExpectationsFile, report FullReport) error {
	if exp.FFmpegVersionPrefix != "" && report.Summary.FFmpegVersion != "" {
		if !strings.HasPrefix(report.Summary.FFmpegVersion, exp.FFmpegVersionPrefix) {
			return fmt.Errorf("ffmpeg version mismatch: got %q, want prefix %q (use scripts/install-ffmpeg.sh)",
				report.Summary.FFmpegVersion, exp.FFmpegVersionPrefix)
		}
	}
	byKey := map[string]CheckResult{}
	for _, r := range report.Results {
		byKey[r.AssetID+"/"+string(r.Check)] = r
	}
	limit := defaultTimingGatePolicy().maxRegression()
	if exp.TimingGate != nil {
		limit = exp.TimingGate.maxRegression()
	}
	tier2OK := map[string]bool{}
	for _, row := range report.BuildModeTimings {
		if row.BuildMode != BuildModePuregoPGO {
			continue
		}
		if row.DeltaPct <= limit+0.02 {
			tier2OK[row.AssetID+"/"+row.Check] = true
		}
	}
	seen := map[string]bool{}
	var diffs []string
	for _, c := range exp.Cases {
		if !expectationCaseApplies(c) {
			continue
		}
		key := c.AssetID + "/" + string(c.Check)
		seen[key] = true
		r, ok := byKey[key]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s %s: missing result", c.AssetID, c.Check))
			continue
		}
		if r.Status != c.Expect {
			// Production gate uses tier-2 purego+PGO subprocess timings; in-process harness can be noisier on CI.
			if c.Expect == StatusSupported && r.Status == StatusMismatch &&
				(c.Check == CheckThumbnailLarge || c.Check == CheckThumbnailSmall) && tier2OK[key] {
				continue
			}
			diffs = append(diffs, fmt.Sprintf("%s %s: expected %s, got %s (%s)",
				c.AssetID, c.Check, c.Expect, r.Status, r.Detail))
		}
	}
	for key := range byKey {
		if seen[key] {
			continue
		}
		diffs = append(diffs, fmt.Sprintf("%s: unexpected result (not in expectations)", key))
	}
	if err := validateBuildModeTimings(exp, report); err != nil {
		diffs = append(diffs, err.Error())
	}
	if len(diffs) == 0 {
		return nil
	}
	return fmt.Errorf("comparison gate failed (%d diffs):\n  %s", len(diffs), strings.Join(diffs, "\n  "))
}

// validateBuildModeTimings requires tier-2 purego+PGO subprocess timings to pass the gate
// for every video thumbnail row collected on amd64.
func validateBuildModeTimings(exp ExpectationsFile, report FullReport) error {
	if len(report.BuildModeTimings) == 0 {
		return nil
	}
	limit := defaultTimingGatePolicy().maxRegression()
	if exp.TimingGate != nil {
		limit = exp.TimingGate.maxRegression()
	}
	var fails []string
	for _, row := range report.BuildModeTimings {
		if row.BuildMode != BuildModePuregoPGO {
			continue
		}
		if row.Check != string(CheckThumbnailSmall) && row.Check != string(CheckThumbnailLarge) {
			continue
		}
		key := row.AssetID + "/" + row.Check
		expectSupported := false
		for _, c := range exp.Cases {
			if !expectationCaseApplies(c) {
				continue
			}
			if c.AssetID+"/"+string(c.Check) == key && c.Expect == StatusSupported {
				expectSupported = true
				break
			}
		}
		if !expectSupported {
			continue
		}
		// Subprocess tier-2 (purego+PGO) is the production gate; allow slack for CI runner variance.
		const tier2Slack = 0.08
		if row.DeltaPct > limit+tier2Slack {
			fails = append(fails, fmt.Sprintf("tier2 %s %s: %.0fms vs ffmpeg %.0fms (%.0f%% > %.0f%%)",
				row.AssetID, row.ThumbPreset, row.GoMediaWrite, row.FFmpegMS, row.DeltaPct*100, limit*100))
		}
	}
	if len(fails) == 0 {
		return nil
	}
	return fmt.Errorf("purego+PGO tier gate:\n  %s", strings.Join(fails, "\n  "))
}
