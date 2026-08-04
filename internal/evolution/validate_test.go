package evolution

import "testing"

func TestValidateProposalSafety(t *testing.T) {
	p := Proposal{SchemaVersion: 1, ID: "p", PatternID: "x", Title: "t", Overlay: "rule", Status: "pending"}
	if !ValidateProposal(p).Valid {
		t.Fatal("expected valid")
	}
	p.Overlay = "write .reasonix/secret"
	if ValidateProposal(p).Valid {
		t.Fatal("expected invalid target")
	}
}

func TestEvaluatePromotionRequiresEvidence(t *testing.T) {
	p := Proposal{SchemaVersion: SchemaVersion, ID: "p", PatternID: "x", Title: "t", Rationale: "evidence", Overlay: "Run the smallest regression test before retrying.", Status: "pending"}
	_, gate := EvaluatePromotion(p)
	if gate.Passed || gate.Code != "insufficient_evidence" {
		t.Fatalf("gate=%+v", gate)
	}
	p.EvidenceCount = 3
	_, gate = EvaluatePromotion(p)
	if !gate.Passed || gate.Code != "passed" {
		t.Fatalf("gate=%+v", gate)
	}
}

func TestEvaluatePromotionRejectsUnsafeProposal(t *testing.T) {
	p := Proposal{SchemaVersion: SchemaVersion, ID: "p", PatternID: "x", Title: "t", Rationale: "evidence", Overlay: ".reasonix/private", Status: "pending", EvidenceCount: 3}
	_, gate := EvaluatePromotion(p)
	if gate.Passed || gate.Code != "offline_validation_failed" {
		t.Fatalf("gate=%+v", gate)
	}
}
