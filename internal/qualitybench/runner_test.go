package qualitybench

import (
	"context"
	"os"
	"path/filepath"
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

func TestExecuteRuntimeDoesNotInventEventsOnProcessFailure(t *testing.T) {
	fixture := Fixture{ID: "runtime", Task: "do task"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := ExecuteRuntime(ctx, fixture, t.TempDir(), "/nonexistent/reasonix", t.TempDir(), "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequiredEffectsMet || len(result.Events) != 0 {
		t.Fatalf("runtime failure was incorrectly qualified: %#v", result)
	}
	if result.Error == "" {
		t.Fatalf("runtime failure did not preserve error: %#v", result)
	}
}

func TestReadEventNamesAcceptsCommonJSONLFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	data := "{\"event\":\"todo_write\"}\n{\"kind\":\"review_report\"}\n{\"name\":\"complete_step\"}\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	events, err := ReadEventNames(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0] != "todo_write" || events[1] != "review_report" || events[2] != "complete_step" {
		t.Fatalf("unexpected events: %#v", events)
	}
}
