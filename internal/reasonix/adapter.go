package reasonix

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// SessionInfo corresponds to a single entry in reasonix session list --json output.
type SessionInfo struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Scope     string `json:"scope"`
	Turns     int    `json:"turns"`
	State     string `json:"state"`
	Recovered bool   `json:"recovered"`
}

// SessionListOutput wraps the full reasonix session list --json response.
type SessionListOutput struct {
	Sessions      []SessionInfo `json:"sessions"`
	SchemaVersion int           `json:"schema_version"`
	Command       string        `json:"command"`
	ExitCode      int           `json:"exit_code"`
	Stderr        string        `json:"stderr,omitempty"`
}

// SessionDetail corresponds to reasonix session show|status --json output.
type SessionDetail struct {
	Session       SessionInfo `json:"session"`
	SchemaVersion int         `json:"schema_version"`
	Command       string      `json:"command"`
	Stderr        string      `json:"stderr,omitempty"`
	ExitCode      int         `json:"exit_code"`
}

// projectRootArg maps an OMR project directory to Reasonix's project-scoped
// session store. --dir has different semantics: it names the store itself.
func (r Runner) projectRootArg() []string {
	if r.ProjectDir == "" {
		return nil
	}
	return []string{"--project-root", r.ProjectDir}
}

// SessionList calls reasonix session list --json and parses the result.
func (r Runner) SessionList(ctx context.Context) (SessionListOutput, error) {
	result := r.Run(ctx, append([]string{"session", "list", "--json"}, r.projectRootArg()...)...)
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
	args := append([]string{"session", "status", branchID, "--json"}, r.projectRootArg()...)
	result := r.Run(ctx, args...)
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
	args := append([]string{"session", "show", branchID, "--json"}, r.projectRootArg()...)
	result := r.Run(ctx, args...)
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
	Event  string `json:"event"`
	Match  string `json:"match,omitempty"`
	Scope  string `json:"scope"`
	Status string `json:"status"`
}

// HookListOutput wraps the reasonix hook list --json response.
type HookListOutput struct {
	Hooks         []HookInfo `json:"hooks"`
	SchemaVersion int        `json:"schema_version"`
	Command       string     `json:"command"`
	ExitCode      int        `json:"exit_code"`
	Stderr        string     `json:"stderr,omitempty"`
}

type HookSource struct {
	Scope     string `json:"scope"`
	Status    string `json:"status"`
	HookCount int    `json:"hook_count"`
}

// HookStatusOutput wraps the reasonix hook status --json response.
// Always populated; errors are captured in Error/Unavailable fields.
type HookStatusOutput struct {
	TrustedProject bool         `json:"trusted_project"`
	ProjectDefines bool         `json:"project_defines"`
	Sources        []HookSource `json:"sources"`
	SchemaVersion  int          `json:"schema_version"`
	Command        string       `json:"command"`
	ExitCode       int          `json:"exit_code"`
	Stderr         string       `json:"stderr,omitempty"`
	Error          string       `json:"error,omitempty"`
	Unavailable    bool         `json:"unavailable,omitempty"`
}

// HookList calls reasonix hook list --json.
func (r Runner) HookList(ctx context.Context) (HookListOutput, error) {
	args := append([]string{"hook", "list", "--json"}, r.projectRootArg()...)
	result := r.Run(ctx, args...)
	out := HookListOutput{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return out, fmt.Errorf("reasonix hook list failed (exit %d): %w", result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return out, fmt.Errorf("parse hook list JSON: %w", err)
	}
	return out, nil
}

// HookStatus calls reasonix hook status --json.
// Always returns a result; errors are captured in Error/Unavailable fields.
func (r Runner) HookStatus(ctx context.Context) HookStatusOutput {
	args := append([]string{"hook", "status", "--json"}, r.projectRootArg()...)
	result := r.Run(ctx, args...)
	out := HookStatusOutput{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		out.Error = fmt.Sprintf("reasonix hook status failed (exit %d): %v", result.ExitCode, result.Err)
		out.Unavailable = true
		return out
	}
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		out.Error = fmt.Sprintf("parse hook status JSON: %v", err)
		out.Unavailable = true
	}
	return out
}

// TaskInfo corresponds to a single entry in reasonix task list --json output.
type TaskInfo struct {
	ID               string `json:"id"`
	SessionID        string `json:"session_id"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at,omitempty"`
	ArtifactComplete bool   `json:"artifact_complete"`
}

// TaskListOutput wraps the reasonix task list --json response.
type TaskListOutput struct {
	Tasks         []TaskInfo `json:"tasks"`
	SchemaVersion int        `json:"schema_version"`
	Command       string     `json:"command"`
	ExitCode      int        `json:"exit_code"`
	Stderr        string     `json:"stderr,omitempty"`
}

// TaskList calls reasonix task list --json, optionally filtered by sessionID.
func (r Runner) TaskList(ctx context.Context, sessionID string) (TaskListOutput, error) {
	args := []string{"task", "list", "--json"}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	args = append(args, r.projectRootArg()...)
	result := r.Run(ctx, args...)
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
	Task          TaskInfo `json:"task"`
	SchemaVersion int      `json:"schema_version"`
	Command       string   `json:"command"`
	ExitCode      int      `json:"exit_code"`
	Stderr        string   `json:"stderr,omitempty"`
}

// TaskShow calls reasonix task show <task-id> --json, optionally with sessionID.
func (r Runner) TaskShow(ctx context.Context, taskID, sessionID string) (TaskDetail, error) {
	args := []string{"task", "show", taskID, "--json"}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	args = append(args, r.projectRootArg()...)
	result := r.Run(ctx, args...)
	detail := TaskDetail{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return detail, fmt.Errorf("reasonix task show %s failed (exit %d): %w", taskID, result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &detail); err != nil {
		return detail, fmt.Errorf("parse task show JSON: %w", err)
	}
	return detail, nil
}

type RecoveryEntry struct {
	SessionID string `json:"session_id"`
	State     string `json:"state"`
	UpdatedAt string `json:"updated_at"`
	Tasks     int    `json:"tasks"`
	Failures  int    `json:"failures"`
	Pending   int    `json:"pending"`
	InFlight  bool   `json:"in_flight"`
}

// RecoveryInfo corresponds to reasonix session recovery --json output.
type RecoveryInfo struct {
	Recoveries    []RecoveryEntry `json:"recoveries"`
	SchemaVersion int             `json:"schema_version"`
	Command       string          `json:"command"`
	ExitCode      int             `json:"exit_code"`
	Stderr        string          `json:"stderr,omitempty"`
}

// SessionRecovery calls reasonix session recovery [<branch-id>] --json.
func (r Runner) SessionRecovery(ctx context.Context, branchID string) (RecoveryInfo, error) {
	args := []string{"session", "recovery"}
	if branchID != "" {
		args = append(args, branchID)
	}
	args = append(args, "--json")
	args = append(args, r.projectRootArg()...)
	result := r.Run(ctx, args...)
	info := RecoveryInfo{ExitCode: result.ExitCode, Stderr: result.Stderr}
	if result.Err != nil {
		return info, fmt.Errorf("reasonix session recovery failed (exit %d): %w", result.ExitCode, result.Err)
	}
	if err := json.Unmarshal([]byte(result.Stdout), &info); err != nil {
		return info, fmt.Errorf("parse session recovery JSON: %w", err)
	}
	return info, nil
}

// RunWithEvents runs a task with structured JSONL events output.
// Reasonix v1.17.20 treats --events-jsonl as a boolean flag (no file argument);
// events are emitted to stdout. This method captures stdout and writes it to
// the specified file for OMR's post-processing.
// On non-zero exit, the stdout JSONL (which includes run_done with ok=false)
// is still saved so callers can inspect the failure events.
func (r Runner) RunWithEvents(ctx context.Context, prompt string, eventsJSONLPath string) Result {
	args := []string{"run", "--events-jsonl", "--", prompt}
	result := r.Run(ctx, args...)
	if err := os.WriteFile(eventsJSONLPath, []byte(result.Stdout), 0o644); err != nil {
		return Result{ExitCode: -1, Err: fmt.Errorf("write events file %s: %w", eventsJSONLPath, err)}
	}
	return result
}
