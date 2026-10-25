package main

// TimingGatePolicy configures thumbnail latency gates (see expectations.json timingGate).
type TimingGatePolicy struct {
	DefaultMaxRegressionFraction float64           `json:"defaultMaxRegressionFraction"`
	RelaxedCases                 []TimingGateRelax `json:"relaxedCases"`
}

// TimingGateRelax allows a higher regression fraction for specific asset/check rows.
type TimingGateRelax struct {
	AssetID                   string      `json:"assetId"`
	Checks                    []CheckName `json:"checks"`
	MaxRegressionFraction     float64     `json:"maxRegressionFraction"`
	Reason                    string      `json:"reason,omitempty"`
}

func defaultTimingGatePolicy() TimingGatePolicy {
	return TimingGatePolicy{DefaultMaxRegressionFraction: maxTimingRegressionPct}
}

func timingGatePolicyFromExpectations(exp ExpectationsFile) TimingGatePolicy {
	if exp.TimingGate == nil {
		return defaultTimingGatePolicy()
	}
	p := *exp.TimingGate
	if p.DefaultMaxRegressionFraction <= 0 {
		p.DefaultMaxRegressionFraction = maxTimingRegressionPct
	}
	return p
}

func (p TimingGatePolicy) maxRegression(assetID string, check CheckName) (limit float64, relaxed bool, reason string) {
	limit = p.DefaultMaxRegressionFraction
	if p.DefaultMaxRegressionFraction <= 0 {
		limit = maxTimingRegressionPct
	}
	def := limit
	for _, r := range p.RelaxedCases {
		if r.AssetID != assetID || r.MaxRegressionFraction <= 0 {
			continue
		}
		for _, c := range r.Checks {
			if c == check {
				if r.MaxRegressionFraction > limit {
					limit = r.MaxRegressionFraction
				}
				return limit, limit > def, r.Reason
			}
		}
	}
	return limit, false, ""
}
