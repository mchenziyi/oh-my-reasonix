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
