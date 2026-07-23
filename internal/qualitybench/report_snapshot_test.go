package qualitybench

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportJSONSnapshot(t *testing.T) {
	r := Report{
		SchemaVersion:  1,
		RunID:          "omr-test-001",
		ExecutionMode:  ExecutionModeReplay,
		FixtureCount:   2,
		EvaluatedCount: 2,
		QualifiedCount: 2,
		QualifiedRate:  1,
		Metrics:        Metrics{PromptTokens: 100, CompletionTokens: 50, Cost: 0.05, Currency: "USD"},
		Evaluations: []Evaluation{
			{FixtureID: "a", QualifiedCompletion: true, RetryCount: 0, ReviewBlockCount: 0},
			{FixtureID: "b", QualifiedCompletion: true, RetryCount: 1, StallReason: "stalled", ReviewBlockCount: 2},
		},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded.SchemaVersion != 1 || decoded.RunID != "omr-test-001" {
		t.Fatalf("round-trip failed: %#v", decoded)
	}
	if decoded.Evaluations[1].StallReason != "stalled" || decoded.Evaluations[1].RetryCount != 1 || decoded.Evaluations[1].ReviewBlockCount != 2 {
		t.Fatalf("evaluation fields not preserved: %#v", decoded.Evaluations[1])
	}
}

func TestMigrateV0Report(t *testing.T) {
	v0 := `{"fixture_count":1,"evaluated_count":1,"qualified_count":1,"qualified_rate":1,"metrics":{"prompt_tokens":10},"evaluations":[{"fixture_id":"f","qualified_completion":true}]}`
	report, err := MigrateReport([]byte(v0))
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if report.SchemaVersion != 0 {
		t.Fatalf("expected schema_version 0 (v0 migrated), got %d", report.SchemaVersion)
	}
	if report.RunID != "v0-migrated" {
		t.Fatalf("expected run_id v0-migrated, got %q", report.RunID)
	}
	if report.FixtureCount != 1 {
		t.Fatalf("expected fixture_count 1, got %d", report.FixtureCount)
	}
}

func TestReportNoSensitiveData(t *testing.T) {
	r := Report{
		SchemaVersion: 1,
		RunID:         "omr-test",
		ExecutionMode: ExecutionModeReplay,
		Metrics:       Metrics{Cost: 0},
		Evaluations:   []Evaluation{{FixtureID: "f", QualifiedCompletion: true}},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	// No API keys, absolute paths, or prompt text in values
	for _, bad := range []string{"API_KEY", "/Users/", "/home/"} {
		if strings.Contains(string(data), bad) {
			t.Fatalf("report contains sensitive data: %q", bad)
		}
	}
}
