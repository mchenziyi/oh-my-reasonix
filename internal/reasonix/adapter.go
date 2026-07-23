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
