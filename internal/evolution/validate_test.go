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
