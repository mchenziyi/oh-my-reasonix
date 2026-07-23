package qualitybench

import (
	"path/filepath"
	"testing"
)

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
	}}, "test-run", ExecutionModeReplay)
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

// ─── OMR-T01: failure categorization tests ───

func TestEvaluateCategorizesPass(t *testing.T) {
	eval := Evaluate(Fixture{ID: "x", Task: "t"}, RunResult{
		HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true,
	})
	if eval.Category != "pass" {
		t.Fatalf("expected pass category, got %q", eval.Category)
	}
}

func TestEvaluateCategorizesInfra(t *testing.T) {
	eval := Evaluate(Fixture{ID: "x", Task: "t"}, RunResult{
		Failed: true, Error: "context deadline exceeded",
	})
	if eval.Category != "infra" {
		t.Fatalf("expected infra category, got %q", eval.Category)
	}
}

func TestEvaluateCategorizesTask(t *testing.T) {
	eval := Evaluate(Fixture{ID: "x", Task: "t"}, RunResult{
		HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: false,
	})
	if eval.Category != "task" {
		t.Fatalf("expected task category, got %q", eval.Category)
	}
}

func TestEvaluateCategorizesTaskTestsSkipped(t *testing.T) {
	eval := Evaluate(Fixture{ID: "x", Task: "t"}, RunResult{
		HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, TestsSkipped: true,
	})
	if eval.Category != "task" {
		t.Fatalf("expected task category for tests skipped, got %q", eval.Category)
	}
}

func TestEvaluateCategorizesJudgment(t *testing.T) {
	eval := Evaluate(Fixture{ID: "x", Task: "t"}, RunResult{
		HiddenTestsPassed: false, RegressionPassed: true, RequiredEffectsMet: true,
	})
	if eval.Category != "judgment" {
		t.Fatalf("expected judgment category, got %q", eval.Category)
	}
}

func TestEvaluateCategorizesModel(t *testing.T) {
	eval := Evaluate(Fixture{ID: "x", Task: "t", ExpectedEvents: []string{"complete_step"}},
		RunResult{HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, Events: nil})
	if eval.Category != "model" {
		t.Fatalf("expected model category, got %q", eval.Category)
	}
}

func TestEvaluateAllIncludesFailedResults(t *testing.T) {
	fixtures := []Fixture{
		{ID: "ok", Task: "t", ExpectedEvents: []string{"e"}},
		{ID: "fail", Task: "t", ExpectedEvents: []string{"e"}},
	}
	results := map[string]RunResult{
		"ok":   {HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, Events: []string{"e"}},
		"fail": {Failed: true, Error: "boom"},
	}
	report := EvaluateAll(fixtures, results, "test-run", ExecutionModeReplay)
	if report.EvaluatedCount != 2 {
		t.Fatalf("expected 2 evaluated, got %d", report.EvaluatedCount)
	}
	if report.QualifiedCount != 1 {
		t.Fatalf("expected 1 qualified, got %d", report.QualifiedCount)
	}
	// Failed fixture should have infra category
	for _, e := range report.Evaluations {
		if e.FixtureID == "fail" && e.Category != "infra" {
			t.Fatalf("expected infra for failed fixture, got %q", e.Category)
		}
	}
}

func TestReplayPairedNativeOMR(t *testing.T) {
	f := Fixture{
		ID: "paired", Task: "t",
		NativeReplay: &ReplaySpec{ChangedPaths: []string{"a.go"}, HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, Events: []string{"e"}},
		OMRReplay:    &ReplaySpec{ChangedPaths: []string{"a.go"}, HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, Events: []string{"e"}, Metrics: Metrics{Cost: 0.1}},
	}
	native, omr, err := ReplayPaired(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(native.ChangedPaths) != 1 {
		t.Fatal("native replay failed")
	}
	if omr.Metrics.Cost != 0.1 {
		t.Fatal("omr replay metrics not loaded")
	}
}

func TestReplayPairedFallsBackToReplay(t *testing.T) {
	f := Fixture{
		ID: "fallback", Task: "t",
		NativeReplay: &ReplaySpec{ChangedPaths: []string{"a.go"}, HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, Events: []string{"e"}},
		Replay:       &ReplaySpec{ChangedPaths: []string{"a.go"}, HiddenTestsPassed: true, RegressionPassed: true, RequiredEffectsMet: true, Events: []string{"e"}},
	}
	_, omr, err := ReplayPaired(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(omr.ChangedPaths) != 1 {
		t.Fatal("omr replay fallback failed")
	}
}

func TestReplayPairedRequiresNative(t *testing.T) {
	_, _, err := ReplayPaired(Fixture{ID: "no-native", Task: "t"})
	if err == nil {
		t.Fatal("expected error when native_replay missing")
	}
}

func TestFullFlowFixtureReplayable(t *testing.T) {
	ids := []string{"full-flow-bug-fix", "full-flow-feature", "full-flow-refactor"}
	for _, id := range ids {
		path := filepath.Join("..", "..", "benchmarks", "fixtures", id, "fixture.yaml")
		f, err := LoadFixture(path)
		if err != nil {
			t.Fatalf("load %s: %v", id, err)
		}
		result, err := Replay(f)
		if err != nil {
			t.Fatalf("replay %s: %v", id, err)
		}
		eval := Evaluate(f, result)
		if !eval.QualifiedCompletion {
			t.Fatalf("%s: not qualified: %v", id, eval.Failures)
		}
		if len(result.ChangedPaths) < 2 {
			t.Fatalf("%s: expected >=2 changed files, got %d", id, len(result.ChangedPaths))
		}
		// Also verify paired replay works for this fixture
		native, omr, pairErr := ReplayPaired(f)
		if pairErr != nil {
			t.Fatalf("%s: paired replay failed: %v", id, pairErr)
		}
		nativeEval := Evaluate(f, native)
		omrEval := Evaluate(f, omr)
		if !nativeEval.QualifiedCompletion {
			t.Fatalf("%s: native paired replay not qualified: %v", id, nativeEval.Failures)
		}
		if !omrEval.QualifiedCompletion {
			t.Fatalf("%s: omr paired replay not qualified: %v", id, omrEval.Failures)
		}
	}
}
