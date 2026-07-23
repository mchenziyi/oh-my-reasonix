package reasonix

import (
	"context"
	"encoding/json"
	"fmt"
)

// SessionInfo corresponds to a single entry in reasonix session list --json output.
// Only includes safe, sanitized fields.
type SessionInfo struct {
	BranchID      string `json:"branch_id"`
	Status        string `json:"status"`
	Scope         string `json:"scope,omitempty"`
	Turn          int    `json:"turn,omitempty"`
	Lifecycle     string `json:"lifecycle,omitempty"`
	Recovered     bool   `json:"recovered,omitempty"`
	SchemaVersion int    `json:"schema_version"`
}

// SessionListOutput wraps the full reasonix session list --json response.
type SessionListOutput struct {
	Sessions      []SessionInfo `json:"sessions"`
	SchemaVersion int           `json:"schema_version"`
	ExitCode      int           `json:"exit_code"`
	Stderr        string        `json:"stderr,omitempty"`
}

// SessionDetail corresponds to reasonix session show|status --json output.
type SessionDetail struct {
	BranchID      string `json:"branch_id"`
	Status        string `json:"status"`
	Scope         string `json:"scope,omitempty"`
	Turn          int    `json:"turn,omitempty"`
	Lifecycle     string `json:"lifecycle,omitempty"`
	Recovered     bool   `json:"recovered,omitempty"`
	SchemaVersion int    `json:"schema_version"`
	Stderr        string `json:"stderr,omitempty"`
	ExitCode      int    `json:"exit_code"`
}

// SessionList calls reasonix session list --json and parses the result.
func (r Runner) SessionList(ctx context.Context) (SessionListOutput, error) {
	result := r.Run(ctx, "session", "list", "--json")
	out := SessionListOutput{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return out, fmt.Errorf("reasonix session list failed (exit %d): %w", result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return out, fmt.Errorf("parse session list JSON: %w", err)
	}
	return out, nil
}

// SessionStatus calls reasonix session status <branch-id> --json.
func (r Runner) SessionStatus(ctx context.Context, branchID string) (SessionDetail, error) {
	result := r.Run(ctx, "session", "status", branchID, "--json")
	detail := SessionDetail{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return detail, fmt.Errorf("reasonix session status failed (exit %d): %w", result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &detail); err != nil {
		return detail, fmt.Errorf("parse session status JSON: %w", err)
	}
	return detail, nil
}

// SessionShow calls reasonix session show <branch-id> --json.
func (r Runner) SessionShow(ctx context.Context, branchID string) (SessionDetail, error) {
	result := r.Run(ctx, "session", "show", branchID, "--json")
	detail := SessionDetail{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return detail, fmt.Errorf("reasonix session show failed (exit %d): %w", result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &detail); err != nil {
		return detail, fmt.Errorf("parse session show JSON: %w", err)
	}
	return detail, nil
}

// HookInfo corresponds to a single entry in reasonix hook list --json output.
type HookInfo struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Source  string `json:"source,omitempty"`
	Matcher string `json:"matcher,omitempty"`
	Event   string `json:"event,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

// HookListOutput wraps the reasonix hook list --json response.
type HookListOutput struct {
	Hooks       []HookInfo `json:"hooks"`
	SchemaVersion int      `json:"schema_version"`
	ExitCode    int        `json:"exit_code"`
	Stderr      string     `json:"stderr,omitempty"`
}

// HookList calls reasonix hook list --json.
func (r Runner) HookList(ctx context.Context) (HookListOutput, error) {
	result := r.Run(ctx, "hook", "list", "--json")
	out := HookListOutput{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return out, fmt.Errorf("reasonix hook list failed (exit %d): %w", result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return out, fmt.Errorf("parse hook list JSON: %w", err)
	}
	return out, nil
}

// TaskInfo corresponds to a single entry in reasonix task list --json output.
type TaskInfo struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id,omitempty"`
	Type      string `json:"type,omitempty"`
	Status    string `json:"status"`
	Step      int    `json:"step,omitempty"`
}

// TaskListOutput wraps the reasonix task list --json response.
type TaskListOutput struct {
	Tasks       []TaskInfo `json:"tasks"`
	SchemaVersion int       `json:"schema_version"`
	ExitCode    int        `json:"exit_code"`
	Stderr      string     `json:"stderr,omitempty"`
}

// TaskList calls reasonix task list --json.
func (r Runner) TaskList(ctx context.Context) (TaskListOutput, error) {
	result := r.Run(ctx, "task", "list", "--json")
	out := TaskListOutput{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return out, fmt.Errorf("reasonix task list failed (exit %d): %w", result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return out, fmt.Errorf("parse task list JSON: %w", err)
	}
	return out, nil
}

// TaskDetail corresponds to reasonix task show <task-id> --json output.
type TaskDetail struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id,omitempty"`
	Type        string `json:"type,omitempty"`
	Status      string `json:"status"`
	Step        int    `json:"step,omitempty"`
	SchemaVersion int  `json:"schema_version"`
	ExitCode    int    `json:"exit_code"`
	Stderr      string `json:"stderr,omitempty"`
}

// TaskShow calls reasonix task show <task-id> --json.
func (r Runner) TaskShow(ctx context.Context, taskID string) (TaskDetail, error) {
	result := r.Run(ctx, "task", "show", taskID, "--json")
	detail := TaskDetail{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return detail, fmt.Errorf("reasonix task show %s failed (exit %d): %w", taskID, result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &detail); err != nil {
		return detail, fmt.Errorf("parse task show JSON: %w", err)
	}
	return detail, nil
}
