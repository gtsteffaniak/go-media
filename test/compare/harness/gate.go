package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ExpectationsFile is the checked-in manifest of allowed comparison outcomes.
type ExpectationsFile struct {
	Version             int                `json:"version"`
	FFmpegVersionPrefix string             `json:"ffmpegVersionPrefix"`
	TimingGate          *TimingGatePolicy  `json:"timingGate,omitempty"`
	Cases               []ExpectationCase  `json:"cases"`
}

// ExpectationCase is one asset/check expected status.
type ExpectationCase struct {
	AssetID string           `json:"assetId"`
	Check   CheckName        `json:"check"`
	Expect  CapabilityStatus `json:"expect"`
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
	seen := map[string]bool{}
	var diffs []string
	for _, c := range exp.Cases {
		key := c.AssetID + "/" + string(c.Check)
		seen[key] = true
		r, ok := byKey[key]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s %s: missing result", c.AssetID, c.Check))
			continue
		}
		if r.Status != c.Expect {
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
	if len(diffs) == 0 {
		return nil
	}
	return fmt.Errorf("comparison gate failed (%d diffs):\n  %s", len(diffs), strings.Join(diffs, "\n  "))
}
