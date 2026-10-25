package main

import "time"

// CapabilityStatus classifies how go-media behaved vs ffmpeg/ffprobe for one check.
type CapabilityStatus string

const (
	StatusSupported          CapabilityStatus = "supported"
	StatusTypedUnsupported   CapabilityStatus = "typed_unsupported"
	StatusMismatch           CapabilityStatus = "mismatch"
	StatusError              CapabilityStatus = "error"
	StatusSkipped            CapabilityStatus = "skipped"
)

// CheckName identifies a go-media API surface under test.
type CheckName string

const (
	CheckDuration   CheckName = "duration"
	CheckProbe      CheckName = "probe"
	CheckTags       CheckName = "tags"
	CheckArtwork    CheckName = "artwork"
	CheckLyrics     CheckName = "lyrics"
	CheckSubtitles  CheckName = "subtitles"
	CheckThumbnailSmall CheckName = "thumbnail_small"
	CheckThumbnailLarge CheckName = "thumbnail_large"
)

// Asset describes one generated fixture file.
type Asset struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Sidecars    []string `json:"sidecars,omitempty"`
}

// CheckResult is one comparison row for an asset and API check.
type CheckResult struct {
	AssetID       string           `json:"assetId"`
	Check         CheckName        `json:"check"`
	Status        CapabilityStatus `json:"status"`
	GoMediaMS     float64          `json:"goMediaMs"`
	GoMediaWriteMS float64         `json:"goMediaWriteMs,omitempty"`
	FFmpegMS      float64          `json:"ffmpegMs,omitempty"`
	// TimingDeltaPct is (goMediaWriteMs - ffmpegMs) / ffmpegMs for supported/mismatch rows only (median of 7 runs).
	TimingDeltaPct *float64 `json:"timingDeltaPct,omitempty"`
	TimingGateFail bool             `json:"timingGateFail,omitempty"`
	// TimingGateLimitPct is the allowed (go+write−ffmpeg)/ffmpeg fraction for this row (default 0.10).
	TimingGateLimitPct float64 `json:"timingGateLimitPct,omitempty"`
	// TimingGateRelaxed is true when this row uses an expectations.json timingGate relaxed case (e.g. MJPEG 20%).
	TimingGateRelaxed bool `json:"timingGateRelaxed,omitempty"`
	TimingGateRelaxReason string `json:"timingGateRelaxReason,omitempty"`
	Detail        string           `json:"detail,omitempty"`
	DurationDelta float64          `json:"durationDeltaSec,omitempty"`
	FieldDiffs    []string         `json:"fieldDiffs,omitempty"`
	PSNR          float64          `json:"psnr,omitempty"`
	SSIM          float64          `json:"ssim,omitempty"`
	PSNRDecode    float64          `json:"psnrDecode,omitempty"`
	SSIMDecode    float64          `json:"ssimDecode,omitempty"`
	ThumbPreset   string           `json:"thumbPreset,omitempty"`
	ThumbGoPath   string           `json:"thumbGoPath,omitempty"`
	ThumbFFPath   string           `json:"thumbFFPath,omitempty"`
	// Previous-run regression (when a prior report.json existed).
	PrevGoMediaMS   *float64 `json:"prevGoMediaMs,omitempty"`
	PrevTimingDelta *float64 `json:"prevTimingDeltaPct,omitempty"`
	TimingRegression *bool   `json:"timingRegression,omitempty"`
	StatusRegression *bool   `json:"statusRegression,omitempty"`
}

// TimingRelaxedRow documents a thumbnail row that passed under a relaxed timing gate.
type TimingRelaxedRow struct {
	AssetID         string  `json:"assetId"`
	Check           string  `json:"check"`
	ThumbPreset     string  `json:"thumbPreset,omitempty"`
	TimingDeltaPct  float64 `json:"timingDeltaPct"`
	TimingGateLimit float64 `json:"timingGateLimitPct"`
	GoMediaWriteMS  float64 `json:"goMediaWriteMs"`
	FFmpegMS        float64 `json:"ffmpegMs"`
	Reason          string  `json:"reason,omitempty"`
}

// TimingAggregate summarizes latency across comparable rows.
type TimingAggregate struct {
	ComparableRows int     `json:"comparableRows"`
	TotalGoMediaMS float64 `json:"totalGoMediaMs"`
	TotalFFmpegMS  float64 `json:"totalFfmpegMs"`
	AvgDeltaPct    float64 `json:"avgTimingDeltaPct"`
	MedianDeltaPct float64 `json:"medianTimingDeltaPct"`
}

// CapabilityBreakdown is supported vs other counts per check name.
type CapabilityBreakdown struct {
	Check     string `json:"check"`
	Supported int    `json:"supported"`
	Other     int    `json:"other"`
	Skipped   int    `json:"skipped"`
}

// PreviousRunSummary captures drift vs the last generated report.
type PreviousRunSummary struct {
	GeneratedAt      time.Time `json:"generatedAt"`
	ComparedChecks   int       `json:"comparedChecks"`
	TimingRegressions int      `json:"timingRegressions"`
	StatusRegressions int      `json:"statusRegressions"`
}

// ReportSummary aggregates dashboard counts.
type ReportSummary struct {
	Assets          int            `json:"assets"`
	TotalChecks     int            `json:"totalChecks"`
	ByStatus        map[string]int `json:"byStatus"`
	ByCheck         map[string]int `json:"byCheckSupported"`
	GeneratedAt     time.Time      `json:"generatedAt"`
	SeedCommit      string         `json:"seedCommit"`
	SeedSHA256      string         `json:"seedSha256"`
	FFmpegVersion   string         `json:"ffmpegVersion"`
	FFprobeVersion  string         `json:"ffprobeVersion"`
	GoMediaModule   string         `json:"goMediaModule"`
	Timing          TimingAggregate           `json:"timing"`
	TimingRelaxed   []TimingRelaxedRow        `json:"timingRelaxed,omitempty"`
	CapabilityChart []CapabilityBreakdown     `json:"capabilityBreakdown"`
	PreviousRun     *PreviousRunSummary       `json:"previousRun,omitempty"`
}

// FullReport is serialized to report_site/data/report.json for the static dashboard.
type FullReport struct {
	Summary          ReportSummary        `json:"summary"`
	Assets           []Asset              `json:"assets"`
	Results          []CheckResult        `json:"results"`
	ScalerBenchmarks []ScalerBenchRow     `json:"scalerBenchmarks,omitempty"`
	BuildModeTimings []BuildModeTimingRow  `json:"buildModeTimings,omitempty"`
}
