package qualitybench

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestExecuteFixtureRunsDeclaredChecks(t *testing.T) {
	command := "true"
	if runtime.GOOS == "windows" {
		command = "ver"
	}
	fixture := Fixture{
		ID:              "runner",
		Task:            "task",
		HiddenTests:     []string{command},
		RegressionTests: []string{command},
		Replay: &ReplaySpec{
			HiddenTestsPassed:  true,
			RegressionPassed:   true,
			RequiredEffectsMet: true,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := ExecuteFixture(ctx, fixture, ".")
	if err != nil {
		t.Fatal(err)
	}
	if !result.HiddenTestsPassed || !result.RegressionPassed {
		t.Fatalf("fixture checks did not pass: %#v", result)
	}
}

func TestExecuteFixtureReportsFailedCheck(t *testing.T) {
	command := "false"
	if runtime.GOOS == "windows" {
		command = "exit /B 1"
	}
	fixture := Fixture{ID: "runner-fail", Task: "task", HiddenTests: []string{command}, Replay: &ReplaySpec{}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := ExecuteFixture(ctx, fixture, ".")
	if err == nil || result.HiddenTestsPassed {
		t.Fatalf("expected failed check: result=%#v err=%v", result, err)
	}
}
