package evolution

import "strings"

// ValidateProposal performs the offline safety gate. It does not claim model
// quality improvement and never touches project files.
func ValidateProposal(p Proposal) Experiment {
	checks := []string{"schema", "status", "overlay-target", "prompt-contract", "safety"}
	valid := p.Validate() == nil && p.Status == "pending" && !strings.Contains(p.Overlay, ".reasonix/") && !strings.ContainsAny(p.Overlay, "\x00")
	return Experiment{SchemaVersion: SchemaVersion, ID: NewID("experiment", p.ID), ProposalID: p.ID, Valid: valid, Checks: checks, CreatedAt: Now()}
}
