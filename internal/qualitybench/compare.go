package qualitybench

type Comparison struct {
	Native                   Report  `json:"native"`
	OMR                      Report  `json:"omr"`
	QualifiedCountDelta      int     `json:"qualified_count_delta"`
	QualifiedRateDelta       float64 `json:"qualified_rate_delta"`
	PromptTokensDelta        int     `json:"prompt_tokens_delta"`
	CacheHitTokensDelta      int     `json:"cache_hit_tokens_delta"`
	CacheMissTokensDelta     int     `json:"cache_miss_tokens_delta"`
	CostDelta                float64 `json:"cost_delta"`
	ReadinessChecksDelta     int     `json:"readiness_checks_delta"`
	ReadinessBlocksDelta     int     `json:"readiness_blocks_delta"`
	ReadinessRecoveriesDelta int     `json:"readiness_recoveries_delta"`
	Passed                   bool    `json:"passed"`
}

func CompareReports(native, omr Report) Comparison {
	return Comparison{
		Native:                   native,
		OMR:                      omr,
		QualifiedCountDelta:      omr.QualifiedCount - native.QualifiedCount,
		QualifiedRateDelta:       omr.QualifiedRate - native.QualifiedRate,
		PromptTokensDelta:        omr.Metrics.PromptTokens - native.Metrics.PromptTokens,
		CacheHitTokensDelta:      omr.Metrics.CacheHitTokens - native.Metrics.CacheHitTokens,
		CacheMissTokensDelta:     omr.Metrics.CacheMissTokens - native.Metrics.CacheMissTokens,
		CostDelta:                omr.Metrics.Cost - native.Metrics.Cost,
		ReadinessChecksDelta:     omr.Metrics.ReadinessChecks - native.Metrics.ReadinessChecks,
		ReadinessBlocksDelta:     omr.Metrics.ReadinessBlocks - native.Metrics.ReadinessBlocks,
		ReadinessRecoveriesDelta: omr.Metrics.ReadinessRecoveries - native.Metrics.ReadinessRecoveries,
		Passed:                   native.QualifiedRate >= 1 && omr.QualifiedRate >= 1,
	}
}
