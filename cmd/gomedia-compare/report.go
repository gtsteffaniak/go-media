package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// writeReport builds summary JSON and copies static site assets into reportDir.
func writeReport(reportDir string, assets []Asset, results [][]CheckResult) error {
	dataDir := filepath.Join(reportDir, "data")
	var prev *FullReport
	prevPath := filepath.Join(dataDir, "report.json")
	if b, err := os.ReadFile(prevPath); err == nil {
		var loaded FullReport
		if json.Unmarshal(b, &loaded) == nil {
			prev = &loaded
		}
	}
	flat := flattenResults(results)
	enrichResults(flat, prev)
	summary := buildSummary(assets, nestedFromFlat(flat))
	report := FullReport{Summary: summary, Assets: assets, Results: flat, ScalerBenchmarks: _scalerBenchmarks, BuildModeTimings: _buildModeTimings}
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

func nestedFromFlat(flat []CheckResult) [][]CheckResult {
	return [][]CheckResult{flat}
}

// timingComparable is true when go-media and ffmpeg both finished a check worth latency comparison.
func timingComparable(r CheckResult) bool {
	if r.FFmpegMS <= 0 {
		return false
	}
	switch r.Status {
	case StatusSupported, StatusMismatch:
		return true
	default:
		return false
	}
}

func timingGateRow(r CheckResult) bool {
	if r.Check != CheckThumbnailSmall && r.Check != CheckThumbnailLarge {
		return false
	}
	return timingComparable(r)
}

func clearNonComparableTiming(r *CheckResult) {
	if timingComparable(*r) {
		return
	}
	r.GoMediaMS = 0
	r.GoMediaWriteMS = 0
	r.FFmpegMS = 0
	r.TimingDeltaPct = nil
	r.TimingGateFail = false
}

func enrichResults(flat []CheckResult, prev *FullReport) {
	prevByKey := map[string]CheckResult{}
	if prev != nil {
		for _, r := range prev.Results {
			prevByKey[resultKey(r)] = r
		}
	}
	var timingRegressions, statusRegressions, compared int
	for i := range flat {
		r := &flat[i]
		// Timing delta % and the support gate apply to thumbnail rows only (median write vs ffmpeg).
		if timingComparable(*r) && timingGateRow(*r) {
			goMs := r.GoMediaMS
			if r.GoMediaWriteMS > 0 {
				goMs = r.GoMediaWriteMS
			}
			pct := (goMs - r.FFmpegMS) / r.FFmpegMS
			r.TimingDeltaPct = &pct
		} else {
			r.TimingDeltaPct = nil
		}
		clearNonComparableTiming(r)
		if pr, ok := prevByKey[resultKey(*r)]; ok {
			compared++
			prevMS := pr.GoMediaMS
			r.PrevGoMediaMS = &prevMS
			if pr.TimingDeltaPct != nil {
				v := *pr.TimingDeltaPct
				r.PrevTimingDelta = &v
			}
			reg := timingComparable(*r) && r.GoMediaMS > prevMS*1.15 && r.GoMediaMS-prevMS > 1
			r.TimingRegression = &reg
			if reg {
				timingRegressions++
			}
			stReg := rankStatus(r.Status) < rankStatus(pr.Status)
			r.StatusRegression = &stReg
			if stReg {
				statusRegressions++
			}
		}
	}
	if prev != nil && compared > 0 {
		_lastPrevSummary = &PreviousRunSummary{
			GeneratedAt:       prev.Summary.GeneratedAt,
			ComparedChecks:    compared,
			TimingRegressions: timingRegressions,
			StatusRegressions: statusRegressions,
		}
	}
}

var _lastPrevSummary *PreviousRunSummary
var _scalerBenchmarks []ScalerBenchRow

func resultKey(r CheckResult) string {
	return r.AssetID + "\x00" + string(r.Check)
}

func rankStatus(st CapabilityStatus) int {
	switch st {
	case StatusSupported:
		return 5
	case StatusTypedUnsupported:
		return 4
	case StatusSkipped:
		return 3
	case StatusMismatch:
		return 2
	default:
		return 1
	}
}

// buildSummary aggregates counts for the dashboard.
func buildSummary(assets []Asset, nested [][]CheckResult) ReportSummary {
	s := ReportSummary{
		Assets:      len(assets),
		GeneratedAt: time.Now().UTC(),
		SeedCommit:  seedID,
		SeedSHA256:  seedSHA256,
		ByStatus:    map[string]int{},
		ByCheck:     map[string]int{},
	}
	s.FFmpegVersion = ffmpegVersionLine()
	s.FFprobeVersion = ffprobeVersionLine()
	s.GoMediaModule = "github.com/gtsteffaniak/go-media/pkg/media"
	s.TimingGateBuild = ShippedTimingGateBuild
	s.BuildModeTiers = []string{
		"1: diagnostic — no PGO",
		"2: production — CGO_ENABLED=0 + -pgo=default.pgo (support gate)",
		"3: diagnostic — production + GOEXPERIMENT=simd (not gated)",
	}
	checkNames := []CheckName{
		CheckDuration, CheckProbe, CheckTags, CheckArtwork,
		CheckLyrics, CheckSubtitles, CheckThumbnailSmall, CheckThumbnailLarge,
	}
	byCheckStatus := map[CheckName]map[CapabilityStatus]int{}
	for _, c := range checkNames {
		byCheckStatus[c] = map[CapabilityStatus]int{}
	}
	var deltaPcts []float64
	var totalGo, totalFF float64
	var avgGoSum, avgFFSum float64
	comparable := 0
	ffmpegActions := 0
	var relaxedRows []TimingRelaxedRow
	for _, group := range nested {
		for _, r := range group {
			s.TotalChecks++
			s.ByStatus[string(r.Status)]++
			if r.Status == StatusSupported {
				s.ByCheck[string(r.Check)]++
			}
			byCheckStatus[r.Check][r.Status]++
			if timingComparable(r) && r.FFmpegMS > 0 {
				ffmpegActions++
				goMs := r.GoMediaMS
				if r.GoMediaWriteMS > 0 {
					goMs = r.GoMediaWriteMS
				}
				avgGoSum += goMs
				avgFFSum += r.FFmpegMS
			}
			if timingGateRow(r) {
				comparable++
				goMs := r.GoMediaMS
				if r.GoMediaWriteMS > 0 {
					goMs = r.GoMediaWriteMS
				}
				totalGo += goMs
				totalFF += r.FFmpegMS
				if r.TimingDeltaPct != nil {
					deltaPcts = append(deltaPcts, *r.TimingDeltaPct)
				}
			}
			if r.TimingGateRelaxed && r.TimingDeltaPct != nil && r.Status == StatusSupported {
				relaxedRows = append(relaxedRows, TimingRelaxedRow{
					AssetID:         r.AssetID,
					Check:           string(r.Check),
					ThumbPreset:     r.ThumbPreset,
					TimingDeltaPct:  *r.TimingDeltaPct,
					TimingGateLimit: r.TimingGateLimitPct,
					GoMediaWriteMS:  r.GoMediaWriteMS,
					FFmpegMS:        r.FFmpegMS,
					Reason:          r.TimingGateRelaxReason,
				})
			}
		}
	}
	s.TimingRelaxed = relaxedRows
	for _, c := range checkNames {
		m := byCheckStatus[c]
		s.CapabilityChart = append(s.CapabilityChart, CapabilityBreakdown{
			Check:     string(c),
			Supported: m[StatusSupported],
			Other:     m[StatusTypedUnsupported] + m[StatusMismatch] + m[StatusError],
			Skipped:   m[StatusSkipped],
		})
	}
	s.Timing = TimingAggregate{
		ComparableRows:   comparable,
		FfmpegActionRows: ffmpegActions,
		TotalGoMediaMS:   totalGo,
		TotalFFmpegMS:    totalFF,
	}
	if ffmpegActions > 0 {
		s.Timing.AvgGoMediaMS = avgGoSum / float64(ffmpegActions)
		s.Timing.AvgFfmpegMS = avgFFSum / float64(ffmpegActions)
	}
	if len(deltaPcts) > 0 {
		var sum float64
		for _, d := range deltaPcts {
			sum += d
		}
		s.Timing.AvgDeltaPct = sum / float64(len(deltaPcts))
		sorted := append([]float64(nil), deltaPcts...)
		sort.Float64s(sorted)
		mid := len(sorted) / 2
		if len(sorted)%2 == 0 {
			s.Timing.MedianDeltaPct = (sorted[mid-1] + sorted[mid]) / 2
		} else {
			s.Timing.MedianDeltaPct = sorted[mid]
		}
	}
	if _lastPrevSummary != nil {
		s.PreviousRun = _lastPrevSummary
		_lastPrevSummary = nil
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
