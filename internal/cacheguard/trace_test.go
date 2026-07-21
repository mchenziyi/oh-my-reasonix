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
	if report.Passed || report.BodyMismatch != 1 {
		t.Fatalf("body rewrite was not rejected: %#v", report)
	}
}
