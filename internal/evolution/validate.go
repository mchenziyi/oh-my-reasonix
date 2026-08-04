package evolution

import "strings"

type PromotionGate struct {
	Passed        bool     `json:"passed"`
	Code          string   `json:"code"`
	EvidenceCount int      `json:"evidence_count"`
	QualityScore  int      `json:"quality_score"`
	SafetyPassed  bool     `json:"safety_passed"`
	Checks        []string `json:"checks"`
}

// ValidateProposal performs the offline safety gate. It does not claim model
// quality improvement and never touches project files.
func ValidateProposal(p Proposal) Experiment {
	checks := []string{"schema", "status", "overlay-target", "prompt-contract", "safety"}
	valid := p.Validate() == nil && p.Status == "pending" && !strings.Contains(p.Overlay, ".reasonix/") && !strings.ContainsAny(p.Overlay, "\x00")
	return Experiment{SchemaVersion: SchemaVersion, ID: NewID("experiment", p.ID), ProposalID: p.ID, Valid: valid, Checks: checks, CreatedAt: Now(), SafetyPassed: valid}
}

func EvaluatePromotion(p Proposal) (Experiment, PromotionGate) {
	experiment := ValidateProposal(p)
	assessment := AssessProposal(p)
	gate := PromotionGate{EvidenceCount: p.EvidenceCount, QualityScore: assessment.Score, SafetyPassed: experiment.Valid, Checks: append([]string(nil), experiment.Checks...)}
	switch {
	case !experiment.Valid:
		gate.Code = "offline_validation_failed"
	case p.EvidenceCount < 3:
		gate.Code = "insufficient_evidence"
	case !assessment.Valid || assessment.Score < 80:
		gate.Code = "quality_below_threshold"
	default:
		gate.Passed = true
		gate.Code = "passed"
	}
	experiment.GateStatus = gate.Code
	experiment.GateReason = gate.Code
	experiment.QualityScore = gate.QualityScore
	experiment.EvidenceCount = gate.EvidenceCount
	experiment.SafetyPassed = gate.SafetyPassed
	experiment.Valid = experiment.Valid && gate.Passed
	return experiment, gate
}
