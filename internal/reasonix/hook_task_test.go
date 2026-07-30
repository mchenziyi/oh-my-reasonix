package reasonix

import (
	"context"
	"testing"
)

func TestHookListParsesOutput(t *testing.T) {
	jsonOutput := `{"schema_version":1,"command":"hook.list","hooks":[{"event":"PreToolUse","match":"Bash","scope":"project","status":"active"}]}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	result, err := r.HookList(context.Background())
	if err != nil {
		t.Fatalf("HookList: %v", err)
	}
	if len(result.Hooks) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(result.Hooks))
	}
	if result.Hooks[0].Event != "PreToolUse" || result.Hooks[0].Match != "Bash" {
		t.Fatalf("unexpected hook: %#v", result.Hooks[0])
	}
}

func TestHookListEmpty(t *testing.T) {
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(`{"hooks":[],"schema_version":1}`, 0),
	}
	result, err := r.HookList(context.Background())
	if err != nil {
		t.Fatalf("HookList: %v", err)
	}
	if len(result.Hooks) != 0 {
		t.Fatalf("expected 0 hooks, got %d", len(result.Hooks))
	}
}

func TestTaskListParsesOutput(t *testing.T) {
	jsonOutput := `{"schema_version":1,"command":"task.list","tasks":[{"id":"task-1","session_id":"session-abc","kind":"subagent","status":"completed","started_at":"2026-07-30T00:00:00Z","finished_at":"2026-07-30T00:01:00Z","artifact_complete":true}]}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	result, err := r.TaskList(context.Background(), "")
	if err != nil {
		t.Fatalf("TaskList: %v", err)
	}
	if len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.Tasks))
	}
	if result.Tasks[0].ID != "task-1" || result.Tasks[0].Kind != "subagent" || !result.Tasks[0].ArtifactComplete {
		t.Fatalf("unexpected task: %#v", result.Tasks[0])
	}
}

func TestTaskListEmpty(t *testing.T) {
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(`{"tasks":[],"schema_version":1}`, 0),
	}
	result, err := r.TaskList(context.Background(), "")
	if err != nil {
		t.Fatalf("TaskList: %v", err)
	}
	if len(result.Tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(result.Tasks))
	}
}

func TestTaskShowParsesOutput(t *testing.T) {
	jsonOutput := `{"schema_version":1,"command":"task.show","task":{"id":"task-1","session_id":"session-abc","kind":"subagent","status":"running","started_at":"2026-07-30T00:00:00Z","artifact_complete":false}}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	detail, err := r.TaskShow(context.Background(), "task-1", "")
	if err != nil {
		t.Fatalf("TaskShow: %v", err)
	}
	if detail.Task.ID != "task-1" {
		t.Fatalf("expected task-1, got %q", detail.Task.ID)
	}
	if detail.Task.SessionID != "session-abc" {
		t.Fatalf("expected session-abc, got %q", detail.Task.SessionID)
	}
}

func TestHookStatusParsesOutput(t *testing.T) {
	jsonOutput := `{"schema_version":1,"command":"hook.status","trusted_project":true,"project_defines":false,"sources":[{"scope":"global","status":"loaded","hook_count":2},{"scope":"project","status":"missing","hook_count":0}]}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	out := r.HookStatus(context.Background())
	if out.Error != "" {
		t.Fatalf("unexpected HookStatus error: %s", out.Error)
	}
	if out.Unavailable {
		t.Fatal("expected available=true")
	}
	if !out.TrustedProject || out.ProjectDefines {
		t.Fatalf("unexpected trust status: %#v", out)
	}
	if len(out.Sources) != 2 || out.Sources[0].HookCount != 2 {
		t.Fatalf("unexpected sources: %#v", out.Sources)
	}
}

func TestHookStatusEmpty(t *testing.T) {
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(`{"schema_version":1,"command":"hook.status","trusted_project":false,"project_defines":false,"sources":[]}`, 0),
	}
	out := r.HookStatus(context.Background())
	if out.Error != "" {
		t.Fatalf("unexpected HookStatus error: %s", out.Error)
	}
	if len(out.Sources) != 0 {
		t.Fatalf("expected empty sources, got %d", len(out.Sources))
	}
}

func TestHookStatusInvalidJSON(t *testing.T) {
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand("not valid json", 0),
	}
	out := r.HookStatus(context.Background())
	if !out.Unavailable {
		t.Fatal("expected unavailable=true for invalid JSON")
	}
	if out.Error == "" {
		t.Fatal("expected error message for invalid JSON")
	}
}

func TestHookStatusNonZeroExit(t *testing.T) {
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand("", 1),
	}
	out := r.HookStatus(context.Background())
	if !out.Unavailable {
		t.Fatal("expected unavailable=true for non-zero exit")
	}
	if out.Error == "" {
		t.Fatal("expected error message for non-zero exit")
	}
}
