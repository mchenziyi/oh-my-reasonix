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
