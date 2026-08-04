package evolution

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type ProposalAssessment struct {
	Valid  bool     `json:"valid"`
	Score  int      `json:"score"`
	Issues []string `json:"issues,omitempty"`
}

// AssessProposal provides a deterministic quality signal for review and
// reporting. It never approves or applies a proposal.
func AssessProposal(p Proposal) ProposalAssessment {
	a := ProposalAssessment{Valid: true, Score: 100}
	if err := p.Validate(); err != nil {
		a.Valid = false
		a.Score -= 60
		a.Issues = append(a.Issues, err.Error())
	}
	if p.Rationale == "" {
		a.Valid = false
		a.Score -= 15
		a.Issues = append(a.Issues, "missing rationale")
	}
	if len(p.Overlay) < 20 {
		a.Valid = false
		a.Score -= 15
		a.Issues = append(a.Issues, "overlay is too short")
	}
	if p.Status != "pending" {
		a.Valid = false
		a.Score -= 10
		a.Issues = append(a.Issues, "proposal is not pending")
	}
	if a.Score < 0 {
		a.Score = 0
	}
	return a
}

// ParseProposal accepts only the documented machine-readable proposal object.
// It deliberately rejects extra fields and never executes or writes its input.
func ParseProposal(data []byte) (Proposal, error) {
	var p Proposal
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Proposal{}, fmt.Errorf("invalid proposal JSON: %w", err)
	}
	if err := p.Validate(); err != nil {
		return Proposal{}, err
	}
	return p, nil
}
