package reasonix

import (
	"context"
	"testing"
)

func TestHookListParsesOutput(t *testing.T) {
	jsonOutput := `{"hooks":[{"name":"pre-commit","status":"active","event":"commit","scope":"local"}],"schema_version":1}`
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
	if result.Hooks[0].Name != "pre-commit" {
		t.Fatalf("expected pre-commit, got %q", result.Hooks[0].Name)
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
	jsonOutput := `{"tasks":[{"id":"task-1","status":"completed","type":"delivery","step":5}],"schema_version":1}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	result, err := r.TaskList(context.Background())
	if err != nil {
		t.Fatalf("TaskList: %v", err)
	}
	if len(result.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(result.Tasks))
	}
	if result.Tasks[0].ID != "task-1" {
		t.Fatalf("expected task-1, got %q", result.Tasks[0].ID)
	}
}

func TestTaskListEmpty(t *testing.T) {
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(`{"tasks":[],"schema_version":1}`, 0),
	}
	result, err := r.TaskList(context.Background())
	if err != nil {
		t.Fatalf("TaskList: %v", err)
	}
	if len(result.Tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(result.Tasks))
	}
}

func TestTaskShowParsesOutput(t *testing.T) {
	jsonOutput := `{"id":"task-1","session_id":"session-abc","status":"running","type":"delivery","step":3,"schema_version":1}`
	r := Runner{
		Binary:         "reasonix",
		commandFactory: mockCommand(jsonOutput, 0),
	}
	detail, err := r.TaskShow(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("TaskShow: %v", err)
	}
	if detail.ID != "task-1" {
		t.Fatalf("expected task-1, got %q", detail.ID)
	}
	if detail.SessionID != "session-abc" {
		t.Fatalf("expected session-abc, got %q", detail.SessionID)
	}
}
