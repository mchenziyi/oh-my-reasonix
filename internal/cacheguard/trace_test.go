package cacheguard

import (
	"encoding/json"
	"testing"
)

func body(messages ...map[string]string) json.RawMessage {
	return mustJSON(map[string]interface{}{"messages": messages})
}

func mustJSON(value interface{}) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func TestAnalyzePrefixStreamsAndAmbiguity(t *testing.T) {
	stream := []RequestRecord{
		{RunID: "run", RequestID: "a", Body: body(map[string]string{"role": "user", "content": "one"})},
		{RunID: "run", RequestID: "b", Body: body(map[string]string{"role": "user", "content": "one"}, map[string]string{"role": "assistant", "content": "two"})},
	}
	report := Analyze(stream)
	if !report.Passed || report.WarmEligible != 1 || report.AmbiguousStream != 0 {
		t.Fatalf("unexpected prefix report: %#v", report)
	}
	ambiguous := Analyze([]RequestRecord{
		{RunID: "run", RequestID: "a", Body: body(map[string]string{"role": "user", "content": "one"})},
		{RunID: "run", RequestID: "b", Body: body(map[string]string{"role": "user", "content": "one"})},
	})
	if ambiguous.Passed || ambiguous.AmbiguousStream != 1 {
		t.Fatalf("duplicate start was not rejected: %#v", ambiguous)
	}
}

func TestAnalyzeRejectsBodyRewrite(t *testing.T) {
	report := Analyze([]RequestRecord{{
		RunID:         "run",
		RequestID:     "a",
		Body:          body(map[string]string{"role": "user", "content": "one"}),
		ForwardedBody: body(map[string]string{"role": "user", "content": "changed"}),
	}})
	if report.Passed || report.BodyMismatch != 1 || report.Records[0].Error == "" {
		t.Fatalf("body rewrite was not rejected: %#v", report)
	}
}

func TestAnalyzeRejectsDivergentBranchWithDiagnostic(t *testing.T) {
	report := Analyze([]RequestRecord{
		{RunID: "run", RequestID: "a", Body: body(map[string]string{"role": "user", "content": "one"})},
		{RunID: "run", RequestID: "b", Body: body(map[string]string{"role": "user", "content": "one"}, map[string]string{"role": "assistant", "content": "two"})},
		{RunID: "run", RequestID: "c", Body: body(map[string]string{"role": "user", "content": "one"}, map[string]string{"role": "assistant", "content": "other"})},
	})
	if report.Passed || report.UnexpectedDivergence != 1 || report.Records[2].Error == "" {
		t.Fatalf("divergent branch was not diagnosed: %#v", report)
	}
}

func TestAnalyzeCountsUsageForColdAndWarmRequests(t *testing.T) {
	report := Analyze([]RequestRecord{
		{RunID: "run", RequestID: "a", Body: body(map[string]string{"role": "user", "content": "one"}), Usage: &Usage{PromptCacheMissTokens: 10}},
		{RunID: "run", RequestID: "b", Body: body(map[string]string{"role": "user", "content": "one"}, map[string]string{"role": "assistant", "content": "two"}), Usage: &Usage{PromptCacheHitTokens: 20}},
	})
	if report.PromptCacheHitTokens != 20 || report.PromptCacheMissTokens != 10 || report.SteadyStateHitRate != 20.0/30.0 {
		t.Fatalf("usage was not aggregated: %#v", report)
	}
}

func TestCompareReportsPreservesBothReportsAndDeltas(t *testing.T) {
	native := Analyze([]RequestRecord{{RunID: "run", RequestID: "a", Body: body(map[string]string{"role": "user", "content": "one"}), Usage: &Usage{PromptCacheMissTokens: 10}}})
	omr := Analyze([]RequestRecord{{RunID: "run", RequestID: "a", Body: body(map[string]string{"role": "user", "content": "one"}), Usage: &Usage{PromptCacheHitTokens: 10}}})
	comparison := CompareReports(native, omr)
	if !comparison.Passed || comparison.PromptCacheHitTokensDelta != 10 || comparison.PromptCacheMissTokensDelta != -10 {
		t.Fatalf("unexpected comparison: %#v", comparison)
	}
	if len(comparison.Native.Records) != 1 || len(comparison.OMR.Records) != 1 {
		t.Fatalf("source reports were not preserved: %#v", comparison)
	}
}
