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
	CheckThumbnail  CheckName = "thumbnail"
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
	FFmpegMS      float64          `json:"ffmpegMs,omitempty"`
	Detail        string           `json:"detail,omitempty"`
	DurationDelta float64          `json:"durationDeltaSec,omitempty"`
	FieldDiffs    []string         `json:"fieldDiffs,omitempty"`
	PSNR          float64          `json:"psnr,omitempty"`
	SSIM          float64          `json:"ssim,omitempty"`
	ThumbGoPath   string           `json:"thumbGoPath,omitempty"`
	ThumbFFPath   string           `json:"thumbFFPath,omitempty"`
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
}

// FullReport is serialized to report_site/data/report.json for the static dashboard.
type FullReport struct {
	Summary ReportSummary `json:"summary"`
	Assets  []Asset       `json:"assets"`
	Results []CheckResult `json:"results"`
}
