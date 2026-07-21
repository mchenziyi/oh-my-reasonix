package cacheguard

// Comparison is a deterministic Native/OMR cache report comparison.
type Comparison struct {
	Native                     Report  `json:"native"`
	OMR                        Report  `json:"omr"`
	PromptCacheHitTokensDelta  int     `json:"prompt_cache_hit_tokens_delta"`
	PromptCacheMissTokensDelta int     `json:"prompt_cache_miss_tokens_delta"`
	WarmEligibleDelta          int     `json:"warm_eligible_delta"`
	SteadyStateHitRateDelta    float64 `json:"steady_state_hit_rate_delta"`
	Passed                     bool    `json:"passed"`
}

// CompareReports keeps both source reports and exposes only arithmetic deltas;
// it never treats a synthetic logical stream ID as a native session ID.
func CompareReports(native, omr Report) Comparison {
	return Comparison{
		Native:                     native,
		OMR:                        omr,
		PromptCacheHitTokensDelta:  omr.PromptCacheHitTokens - native.PromptCacheHitTokens,
		PromptCacheMissTokensDelta: omr.PromptCacheMissTokens - native.PromptCacheMissTokens,
		WarmEligibleDelta:          omr.WarmEligible - native.WarmEligible,
		SteadyStateHitRateDelta:    omr.SteadyStateHitRate - native.SteadyStateHitRate,
		Passed:                     native.Passed && omr.Passed,
	}
}
