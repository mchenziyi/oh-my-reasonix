package qualitybench

import (
	"strings"
	"testing"
)

func TestEvaluateRuleSourcesOrder(t *testing.T) {
	f := Fixture{
		ID:                  "t",
		ExpectedRuleSources: []string{"a.md", "b.md"},
	}
	result := RunResult{
		HiddenTestsPassed:   true,
		RegressionPassed:    true,
		RequiredEffectsMet:  true,
		RuleSources:         []string{"a.md", "b.md"},
	}
	eval := Evaluate(f, result)
	if !eval.QualifiedCompletion {
		t.Fatalf("expected qualified, got failures: %v", eval.Failures)
	}
}

func TestEvaluateRuleSourcesOrderViolation(t *testing.T) {
	f := Fixture{
		ID:                  "t",
		ExpectedRuleSources: []string{"a.md", "b.md"},
	}
	result := RunResult{
		HiddenTestsPassed:   true,
		RegressionPassed:    true,
		RequiredEffectsMet:  true,
		RuleSources:         []string{"b.md", "a.md"},
	}
	eval := Evaluate(f, result)
	if eval.QualifiedCompletion {
		t.Fatal("expected violation to cause unqualified")
	}
	found := false
	for _, fail := range eval.Failures {
		if strings.Contains(fail, "order violation") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected order violation failure, got: %v", eval.Failures)
	}
}

func TestEvaluateRuleSourcesMissing(t *testing.T) {
	f := Fixture{
		ID:                  "t",
		ExpectedRuleSources: []string{"a.md", "b.md"},
	}
	result := RunResult{
		HiddenTestsPassed:   true,
		RegressionPassed:    true,
		RequiredEffectsMet:  true,
		RuleSources:         []string{"a.md"},
	}
	eval := Evaluate(f, result)
	if eval.QualifiedCompletion {
		t.Fatal("expected missing source to cause unqualified")
	}
}

func TestEvaluateForbiddenRuleSources(t *testing.T) {
	f := Fixture{
		ID:                   "t",
		ForbiddenRuleSources: []string{"danger.md"},
	}
	result := RunResult{
		HiddenTestsPassed:   true,
		RegressionPassed:    true,
		RequiredEffectsMet:  true,
		RuleSources:         []string{"safe.md", "danger.md"},
	}
	eval := Evaluate(f, result)
	if eval.QualifiedCompletion {
		t.Fatal("expected forbidden source to cause unqualified")
	}
}

func TestEvaluateExpectedConflictLog(t *testing.T) {
	f := Fixture{
		ID:                  "t",
		ExpectedConflictLog: []string{"resolved: a vs b"},
	}
	result := RunResult{
		HiddenTestsPassed:    true,
		RegressionPassed:     true,
		RequiredEffectsMet:   true,
		ConflictResolutions:  []string{"resolved: a vs b"},
	}
	eval := Evaluate(f, result)
	if !eval.QualifiedCompletion {
		t.Fatalf("expected qualified with conflict log, got: %v", eval.Failures)
	}
}

func TestEvaluateMissingConflictLog(t *testing.T) {
	f := Fixture{
		ID:                  "t",
		ExpectedConflictLog: []string{"resolved: a vs b"},
	}
	result := RunResult{
		HiddenTestsPassed:   true,
		RegressionPassed:    true,
		RequiredEffectsMet:  true,
	}
	eval := Evaluate(f, result)
	if eval.QualifiedCompletion {
		t.Fatal("expected missing conflict resolution to cause unqualified")
	}
}

func TestReplayCopiesRuleData(t *testing.T) {
	f := Fixture{
		ID: "t",
		Replay: &ReplaySpec{
			ChangedPaths:       []string{"a.go"},
			HiddenTestsPassed:  true,
			RegressionPassed:   true,
			RequiredEffectsMet: true,
			Events:             []string{"e"},
			RuleSources:        []string{"r1.md", "r2.md"},
			ConflictResolutions: []string{"c1"},
		},
	}
	result, err := Replay(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.RuleSources) != 2 || result.RuleSources[0] != "r1.md" {
		t.Fatalf("expected 2 rule sources, got %v", result.RuleSources)
	}
	if len(result.ConflictResolutions) != 1 || result.ConflictResolutions[0] != "c1" {
		t.Fatalf("expected 1 conflict resolution, got %v", result.ConflictResolutions)
	}
}
