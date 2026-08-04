package evolution

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

// RollbackFunc restores the effective prompt and manifest after the overlay
// snapshot has been restored.
type RollbackFunc func(proposalID string) error

type Proposer interface {
	Propose(pattern Pattern) (Proposal, error)
}

// RecordRun records only a sanitized summary. Evolution failures are returned
// to the caller so the run command can report them without changing its result.
func RecordRun(store Store, prompt string, result reasonix.Result, stream reasonix.EventStream) error {
	return recordRun(store, prompt, result, stream, nil)
}

func RecordRunWithProposer(store Store, prompt string, result reasonix.Result, stream reasonix.EventStream, proposer Proposer) error {
	return recordRun(store, prompt, result, stream, proposer)
}

func recordRun(store Store, prompt string, result reasonix.Result, stream reasonix.EventStream, proposer Proposer) error {
	class := "success"
	if result.Err != nil || result.ExitCode != 0 {
		class = "task_failure"
	}
	taskClass := "general"
	if len(prompt) > 80 {
		taskClass = prompt[:80]
	}
	nonce := stream.SessionID
	if nonce == "" {
		nonce = Now()
	}
	id := NewID("episode", fmt.Sprintf("%s|%s|%d", taskClass, class, result.ExitCode)) + "_" + NewID("run", nonce)
	e := Episode{SchemaVersion: SchemaVersion, ID: id, TaskClass: taskClass, FailureClass: class, Succeeded: class == "success", SessionID: stream.SessionID, ExitCode: result.ExitCode, PromptTokens: stream.TotalTokens, CreatedAt: Now()}
	if err := store.RecordEpisode(e); err != nil {
		return err
	}
	episodes, err := store.ListEpisodes()
	if err != nil {
		return err
	}
	if pattern := DetectPattern(episodes, 3); pattern != nil {
		patterns, _ := store.ListPatterns()
		for _, existing := range patterns {
			if existing.ID == pattern.ID {
				return nil
			}
		}
		if err := store.SavePattern(*pattern); err != nil {
			return err
		}
		overlay := "When this task fails, inspect the failure evidence and run the smallest relevant regression test before retrying."
		h := sha256.Sum256([]byte(overlay))
		proposal := Proposal{SchemaVersion: SchemaVersion, ID: NewID("proposal", pattern.ID), PatternID: pattern.ID, Title: "Improve handling of " + pattern.FailureClass, Rationale: "Repeated failures were observed; review this conservative suggestion before enabling it.", Overlay: overlay, ContentSHA256: hex.EncodeToString(h[:]), Status: "pending", CreatedAt: Now(), UpdatedAt: Now()}
		if proposer != nil {
			generated, generateErr := proposer.Propose(*pattern)
			if generateErr != nil {
				return generateErr
			}
			proposal = generated
			proposal.SchemaVersion = SchemaVersion
			proposal.PatternID = pattern.ID
			proposal.Status = "pending"
			proposal.UpdatedAt = Now()
		}
		if err := store.SaveProposal(proposal); err != nil {
			return err
		}
	}
	return nil
}

// ObserveApproved applies the conservative observation policy: after an
// approved proposal, two subsequent failed episodes trigger a rollback. The
// episode store is the source of truth; no model judgement is involved.
func ObserveApproved(store Store, rollback RollbackFunc) ([]string, error) {
	proposals, err := store.ListProposals()
	if err != nil {
		return nil, err
	}
	episodes, err := store.ListEpisodes()
	if err != nil {
		return nil, err
	}
	var rolledBack []string
	for _, proposal := range proposals {
		if proposal.Status != "approved" || proposal.ApprovedAt == "" {
			continue
		}
		failures := 0
		for _, episode := range episodes {
			if episode.CreatedAt > proposal.ApprovedAt && !episode.Succeeded && episode.FailureClass != "" {
				failures++
			}
		}
		if failures < 2 {
			continue
		}
		if rollback != nil {
			if err := rollback(proposal.ID); err != nil {
				return rolledBack, err
			}
		}
		proposal.Status = "rolled_back"
		proposal.RollbackReason = "two failed episodes during observation"
		proposal.UpdatedAt = Now()
		if err := store.SaveProposal(proposal); err != nil {
			return rolledBack, err
		}
		rolledBack = append(rolledBack, proposal.ID)
	}
	return rolledBack, nil
}
