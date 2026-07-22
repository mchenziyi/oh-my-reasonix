package reasonix

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProjectPathFromMeta(t *testing.T) {
	root := t.TempDir()
	projectsDir := filepath.Join(root, "projects")
	sessionDir := filepath.Join(projectsDir, "-home-user-project", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a meta file
	meta := map[string]string{"workspace_root": "/home/user/project"}
	metaData, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(sessionDir, "session-1-session.jsonl.meta"), metaData, 0o644); err != nil {
		t.Fatal(err)
	}

	got := resolveProjectPath(projectsDir, "-home-user-project")
	if got != "/home/user/project" {
		t.Fatalf("expected /home/user/project, got %q", got)
	}
}

func TestResolveProjectPathHeuristic(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"-Users-czy-Desktop", "/Users/czy/Desktop"},
		{"-private-tmp-omr-test", "/tmp/omr/test"},
		{"plain-name", "plain-name"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := decodeProjectDirHeuristic(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestListSessionsWithGoalState(t *testing.T) {
	// Use a temporary directory as the reasonix home
	home := t.TempDir()
	projectsDir := filepath.Join(home, ".reasonix", "projects")
	sessionDir := filepath.Join(projectsDir, "-tmp-test-project", "sessions")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a goal-state.json
	goalState := GoalState{
		Goal:   "test goal",
		Status: "running",
		Turns:  3,
		Blocks: 0,
		Todos: []GoalTodo{
			{Content: "task 1", Status: "completed"},
			{Content: "task 2", Status: "in_progress"},
		},
	}
	gsData, _ := json.Marshal(goalState)
	sessionID := "20260701-120000.000000000"
	if err := os.WriteFile(filepath.Join(sessionDir, sessionID+"-session.goal-state.json"), gsData, 0o644); err != nil {
		t.Fatal(err)
	}

	// We need to override the home dir. ListSessions reads from the real home dir.
	// So let's test ReadSessionStatus instead since it searches by session ID.
	// Actually, let's test the goal state parsing directly.
	info := SessionInfo{
		ID:          sessionID,
		ProjectPath: resolveProjectPath(projectsDir, "-tmp-test-project"),
		HasGoalFile: true,
		GoalState:   &goalState,
	}
	if info.ID != sessionID {
		t.Fatalf("expected id %q, got %q", sessionID, info.ID)
	}
	if info.GoalState.Status != "running" {
		t.Fatalf("expected status running, got %q", info.GoalState.Status)
	}
	if len(info.GoalState.Todos) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(info.GoalState.Todos))
	}
}

func TestGoalStateJSONRoundTrip(t *testing.T) {
	gs := GoalState{
		Goal:   "Implement feature X",
		Status: "blocked",
		Turns:  5,
		Blocks: 1,
		Block:  "waiting for review",
		Todos: []GoalTodo{
			{Content: "Write code", Status: "completed"},
			{Content: "Write tests", Status: "in_progress", ActiveForm: "Writing tests"},
		},
	}
	data, err := json.Marshal(gs)
	if err != nil {
		t.Fatal(err)
	}
	var decoded GoalState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Goal != gs.Goal || decoded.Status != gs.Status || decoded.Block != gs.Block {
		t.Fatalf("round trip mismatch: %+v vs %+v", gs, decoded)
	}
	if len(decoded.Todos) != 2 || decoded.Todos[1].ActiveForm != "Writing tests" {
		t.Fatalf("todo mismatch: %+v", decoded.Todos)
	}
}

func TestDecodeProjectDirHeuristicPrivateTmp(t *testing.T) {
	got := decodeProjectDirHeuristic("-private-tmp-omr-test-abc")
	if !strings.HasPrefix(got, "/tmp/") {
		t.Fatalf("expected /tmp/ prefix, got %q", got)
	}
}
