package evolution

import "testing"

func TestAssessProposalIsDeterministic(t *testing.T) {
	p := Proposal{SchemaVersion: SchemaVersion, ID: "p", PatternID: "pattern", Title: "Improve", Rationale: "because", Overlay: "Run the smallest regression test after a failure.", Status: "pending"}
	a := AssessProposal(p)
	if !a.Valid || a.Score != 100 || len(a.Issues) != 0 {
		t.Fatalf("assessment=%+v", a)
	}
	p.Overlay = "x"
	a = AssessProposal(p)
	if a.Valid || a.Score >= 100 || len(a.Issues) == 0 {
		t.Fatalf("expected invalid assessment=%+v", a)
	}
}

func TestParseProposalStrict(t *testing.T) {
	_, err := ParseProposal([]byte(`{"schema_version":1,"id":"p","pattern_id":"x","title":"t","rationale":"r","overlay":"rule","status":"pending","content_sha256":"","created_at":"now","updated_at":"now"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseProposal([]byte(`{"schema_version":1,"id":"p","pattern_id":"x","title":"t","overlay":"rule","status":"pending","extra":1}`)); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}
