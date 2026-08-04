package evolution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	SchemaVersion = 1
	ModeSuggest   = "suggest"
	ModeDisabled  = "disabled"
)

type Episode struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	TaskClass     string `json:"task_class"`
	FailureClass  string `json:"failure_class,omitempty"`
	Succeeded     bool   `json:"succeeded"`
	SessionID     string `json:"session_id,omitempty"`
	ExitCode      int    `json:"exit_code"`
	PromptTokens  int    `json:"prompt_tokens,omitempty"`
	OutputTokens  int    `json:"output_tokens,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type Pattern struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	TaskClass     string   `json:"task_class"`
	FailureClass  string   `json:"failure_class"`
	EpisodeIDs    []string `json:"episode_ids"`
	CreatedAt     string   `json:"created_at"`
}

type Proposal struct {
	SchemaVersion  int    `json:"schema_version"`
	ID             string `json:"id"`
	PatternID      string `json:"pattern_id"`
	Title          string `json:"title"`
	Rationale      string `json:"rationale"`
	Overlay        string `json:"overlay"`
	Status         string `json:"status"`
	ContentSHA256  string `json:"content_sha256"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	ApprovedAt     string `json:"approved_at,omitempty"`
	RollbackReason string `json:"rollback_reason,omitempty"`
}

type Experiment struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	ProposalID    string   `json:"proposal_id"`
	Valid         bool     `json:"valid"`
	Checks        []string `json:"checks"`
	CreatedAt     string   `json:"created_at"`
}

func NewID(prefix string, content string) string {
	h := sha256.Sum256([]byte(content))
	return prefix + "_" + hex.EncodeToString(h[:])[:16]
}

func (e Episode) Validate() error {
	if e.SchemaVersion != SchemaVersion || e.ID == "" || e.TaskClass == "" || e.CreatedAt == "" {
		return fmt.Errorf("invalid episode")
	}
	if !e.Succeeded && e.FailureClass == "" {
		return fmt.Errorf("failed episode requires failure_class")
	}
	return nil
}

func (p Pattern) Validate() error {
	if p.SchemaVersion != SchemaVersion || p.ID == "" || p.TaskClass == "" || p.FailureClass == "" || len(p.EpisodeIDs) < 3 {
		return fmt.Errorf("invalid pattern")
	}
	return nil
}

func (p Proposal) Validate() error {
	if p.SchemaVersion != SchemaVersion || p.ID == "" || p.PatternID == "" || p.Title == "" || p.Overlay == "" {
		return fmt.Errorf("invalid proposal")
	}
	if p.Status != "pending" && p.Status != "approved" && p.Status != "rejected" && p.Status != "rolled_back" {
		return fmt.Errorf("invalid proposal status")
	}
	if strings.ContainsAny(p.Overlay, "\x00") || strings.Contains(strings.ToLower(p.Overlay), "api_key") {
		return fmt.Errorf("proposal contains forbidden content")
	}
	if p.ContentSHA256 != "" {
		h := sha256.Sum256([]byte(p.Overlay))
		if hex.EncodeToString(h[:]) != p.ContentSHA256 {
			return fmt.Errorf("proposal content hash mismatch")
		}
	}
	return nil
}
func (e Experiment) Validate() error {
	if e.SchemaVersion != SchemaVersion || e.ID == "" || e.ProposalID == "" || e.CreatedAt == "" {
		return fmt.Errorf("invalid experiment")
	}
	return nil
}

func Encode(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
