package qualitybench

import "testing"

func TestEvaluateQualifiedCompletion(t *testing.T) {
	fixture := Fixture{ID: "x", Task: "task", AllowedPaths: []string{"src/*"}, ForbiddenPaths: []string{"secret/*"}, ExpectedEvents: []string{"complete_step"}}
	passed := Evaluate(fixture, RunResult{ChangedPaths: []string{"src/main.go"}, HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, Events: []string{"complete_step"}})
	if !passed.QualifiedCompletion {
		t.Fatalf("expected pass: %#v", passed)
	}
	failed := Evaluate(fixture, RunResult{ChangedPaths: []string{"secret/key"}, HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true})
	if failed.QualifiedCompletion || len(failed.Failures) == 0 {
		t.Fatalf("expected failure: %#v", failed)
	}
}

func TestEvaluateReportsRuntimeError(t *testing.T) {
	evaluation := Evaluate(Fixture{ID: "runtime", Task: "task"}, RunResult{
		HiddenTestsPassed:  true,
		RegressionPassed:   true,
		RequiredEffectsMet: true,
		Error:              "reasonix exited with status 1",
	})
	if evaluation.QualifiedCompletion {
		t.Fatalf("expected runtime error to fail qualification: %#v", evaluation)
	}
	if len(evaluation.Failures) != 1 || evaluation.Failures[0] != "runtime error: reasonix exited with status 1" {
		t.Fatalf("runtime error was not reported: %#v", evaluation)
	}
}

func TestReplayUsesDeterministicOutcome(t *testing.T) {
	fixture := Fixture{
		ID:   "m0",
		Task: "task",
		Replay: &ReplaySpec{
			ChangedPaths:       []string{"src/main.go"},
			HiddenTestsPassed:  true,
			RegressionPassed:   true,
			RequiredEffectsMet: true,
			Events:             []string{"omr-explore", "review", "complete_step"},
		},
	}
	result, err := Replay(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if got := Evaluate(fixture, result); !got.QualifiedCompletion {
		t.Fatalf("expected replay to qualify: %#v", got)
	}
}

func TestReplayRequiresOutcome(t *testing.T) {
	_, err := Replay(Fixture{ID: "m0", Task: "task"})
	if err == nil {
		t.Fatal("expected missing replay outcome to fail")
	}
}

func TestCheckGateRejectsMissingFixtureResults(t *testing.T) {
	err := CheckGate(Report{FixtureCount: 2, EvaluatedCount: 1, QualifiedCount: 1, QualifiedRate: 1}, 1)
	if err == nil {
		t.Fatal("expected missing fixture result to fail perfect-rate gate")
	}
}

func TestCheckGateRejectsLowRate(t *testing.T) {
	err := CheckGate(Report{FixtureCount: 2, EvaluatedCount: 2, QualifiedCount: 1, QualifiedRate: 0.5}, 1)
	if err == nil {
		t.Fatal("expected low qualified rate to fail")
	}
}

func TestCheckCostGate(t *testing.T) {
	if err := CheckCostGate(Report{Metrics: Metrics{Cost: 1.2}}, 1); err == nil {
		t.Fatal("expected cost gate failure")
	}
	if err := CheckCostGate(Report{Metrics: Metrics{Cost: 1.2}}, 0); err != nil {
		t.Fatalf("zero budget should disable cost gate: %v", err)
	}
}

func TestEvaluateAllAggregatesMetrics(t *testing.T) {
	fixtures := []Fixture{{ID: "a", Task: "a"}}
	report := EvaluateAll(fixtures, map[string]RunResult{"a": {
		HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true,
		Metrics: Metrics{PromptTokens: 10, CacheHitTokens: 4, Cost: 0.25, Currency: "USD", ReadinessChecks: 3, ReadinessBlocks: 1, ReadinessRecoveries: 1},
	}})
	if report.Metrics.PromptTokens != 10 || report.Metrics.CacheHitTokens != 4 || report.Metrics.Cost != 0.25 || report.Metrics.Currency != "USD" || report.Metrics.ReadinessChecks != 3 || report.Metrics.ReadinessBlocks != 1 || report.Metrics.ReadinessRecoveries != 1 {
		t.Fatalf("metrics were not aggregated: %#v", report.Metrics)
	}
}

func TestCompareReports(t *testing.T) {
	native := Report{QualifiedCount: 1, QualifiedRate: 1, Metrics: Metrics{PromptTokens: 10, CacheHitTokens: 4, Cost: 0.1, ReadinessBlocks: 2, ReadinessRecoveries: 1}}
	omr := Report{QualifiedCount: 1, QualifiedRate: 1, Metrics: Metrics{PromptTokens: 12, CacheHitTokens: 8, Cost: 0.2, ReadinessBlocks: 1, ReadinessRecoveries: 3}}
	comparison := CompareReports(native, omr)
	if !comparison.Passed || comparison.PromptTokensDelta != 2 || comparison.CacheHitTokensDelta != 4 || comparison.CostDelta != 0.1 || comparison.ReadinessBlocksDelta != -1 || comparison.ReadinessRecoveriesDelta != 2 {
		t.Fatalf("unexpected comparison: %#v", comparison)
	}
}

func TestMatchesSupportsGlobstarPaths(t *testing.T) {
	if !Matches(".reasonix/omr/manifest.lock.yaml", ".reasonix/**") {
		t.Fatal("globstar should match nested paths")
	}
	if !Matches("internal/cacheguard/trace.go", "**/*.go") {
		t.Fatal("globstar should match files at any depth")
	}
}
