package main

// TimingGatePolicy configures thumbnail latency gates (see expectations.json timingGate).
type TimingGatePolicy struct {
	DefaultMaxRegressionFraction float64 `json:"defaultMaxRegressionFraction"`
	TargetMaxRegressionFraction  float64 `json:"targetMaxRegressionFraction"`
	// EnforcedAssetIDs is deprecated: when empty, every thumbnail_large/small check applies the gate.
	EnforcedAssetIDs []string `json:"enforcedAssetIds,omitempty"`
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

func (p TimingGatePolicy) enforced(check CheckName) bool {
	return check == CheckThumbnailLarge || check == CheckThumbnailSmall
}

func (p TimingGatePolicy) maxRegression() float64 {
	if p.DefaultMaxRegressionFraction <= 0 {
		return maxTimingRegressionPct
	}
	return p.DefaultMaxRegressionFraction
}
