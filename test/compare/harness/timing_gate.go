package main

import "strings"

// TimingGatePolicy configures thumbnail latency gates (see expectations.json timingGate).
type TimingGatePolicy struct {
	DefaultMaxRegressionFraction float64  `json:"defaultMaxRegressionFraction"`
	TargetMaxRegressionFraction  float64  `json:"targetMaxRegressionFraction"`
	EnforcedAssetIDs             []string `json:"enforcedAssetIds"`
}

func defaultTimingGatePolicy() TimingGatePolicy {
	return TimingGatePolicy{
		DefaultMaxRegressionFraction: maxTimingRegressionPct,
		TargetMaxRegressionFraction:  targetTimingRegressionPct,
	}
}

func timingGatePolicyFromExpectations(exp ExpectationsFile) TimingGatePolicy {
	if exp.TimingGate == nil {
		return defaultTimingGatePolicy()
	}
	p := *exp.TimingGate
	if p.DefaultMaxRegressionFraction <= 0 {
		p.DefaultMaxRegressionFraction = maxTimingRegressionPct
	}
	if p.TargetMaxRegressionFraction <= 0 {
		p.TargetMaxRegressionFraction = targetTimingRegressionPct
	}
	return p
}

func (p TimingGatePolicy) enforced(assetID string) bool {
	if len(p.EnforcedAssetIDs) == 0 {
		return true
	}
	for _, id := range p.EnforcedAssetIDs {
		if id == assetID {
			return true
		}
	}
	return false
}

func (p TimingGatePolicy) maxRegression() float64 {
	if p.DefaultMaxRegressionFraction <= 0 {
		return maxTimingRegressionPct
	}
	return p.DefaultMaxRegressionFraction
}

// mp4ThumbnailAsset is true for MP4 fixtures that use the shared thumbnail/PGO/SIMD stack.
func mp4ThumbnailAsset(id string) bool {
	if id == "mjpeg_mp4" {
		return true
	}
	return strings.HasPrefix(id, "mp4_h264")
}
