package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// writeReport builds summary JSON and copies static site assets into reportDir.
func writeReport(reportDir string, assets []Asset, results [][]CheckResult) error {
	summary := buildSummary(assets, results)
	report := FullReport{Summary: summary, Assets: assets, Results: flattenResults(results)}
	dataDir := filepath.Join(reportDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dataDir, "report.json"), payload, 0o644); err != nil {
		return err
	}
	src := filepath.Join(filepath.Dir(reportDir), "report_site")
	if _, err := os.Stat(src); err != nil {
		src = "report_site"
	}
	return copyReportSite(src, reportDir)
}

func flattenResults(nested [][]CheckResult) []CheckResult {
	var flat []CheckResult
	for _, group := range nested {
		flat = append(flat, group...)
	}
	return flat
}

// buildSummary aggregates counts for the dashboard.
func buildSummary(assets []Asset, nested [][]CheckResult) ReportSummary {
	s := ReportSummary{
		Assets:      len(assets),
		GeneratedAt: time.Now().UTC(),
		SeedCommit:  goFFmpegSeedCommit,
		SeedSHA256:  seedSHA256,
		ByStatus:    map[string]int{},
		ByCheck:     map[string]int{},
	}
	s.FFmpegVersion = ffmpegVersionLine()
	s.FFprobeVersion = ffprobeVersionLine()
	s.GoMediaModule = "github.com/gtsteffaniak/go-media"
	for _, group := range nested {
		for _, r := range group {
			s.TotalChecks++
			s.ByStatus[string(r.Status)]++
			if r.Status == StatusSupported {
				s.ByCheck[string(r.Check)]++
			}
		}
	}
	return s
}

// copyReportSite copies embedded static HTML/CSS/JS next to generated data.
func copyReportSite(srcRoot, destRoot string) error {
	entries, err := os.ReadDir(srcRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() == "data" {
			continue
		}
		src := filepath.Join(srcRoot, e.Name())
		dst := filepath.Join(destRoot, e.Name())
		if e.IsDir() {
			if err := copyDir(src, dst); err != nil {
				return err
			}
			continue
		}
		in, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		if err := os.WriteFile(dst, in, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyDir(s, d); err != nil {
				return err
			}
			continue
		}
		b, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// printSummary logs a short CLI summary after a run.
func printSummary(report FullReport) {
	s := report.Summary
	fmt.Printf("\n=== go-media comparison summary ===\n")
	fmt.Printf("Assets: %d  Checks: %d\n", s.Assets, s.TotalChecks)
	order := []CapabilityStatus{StatusSupported, StatusTypedUnsupported, StatusMismatch, StatusError, StatusSkipped}
	for _, st := range order {
		if v := s.ByStatus[string(st)]; v > 0 {
			fmt.Printf("  %s: %d\n", st, v)
		}
	}
	if n := s.ByStatus[string(StatusSkipped)]; n > 0 {
		fmt.Printf("  (skipped = N/A checks: tags on video, artwork without cover, lyrics without .lrc, subtitles on non-sub fixtures, thumbnail on audio)\n")
	}
	fmt.Printf("Report JSON: report_site/data/report.json\n")
}
