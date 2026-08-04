package evolution

import "testing"

func TestParseProposalStrict(t *testing.T) {
	_, err := ParseProposal([]byte(`{"schema_version":1,"id":"p","pattern_id":"x","title":"t","rationale":"r","overlay":"rule","status":"pending","content_sha256":"","created_at":"now","updated_at":"now"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseProposal([]byte(`{"schema_version":1,"id":"p","pattern_id":"x","title":"t","overlay":"rule","status":"pending","extra":1}`)); err == nil {
		t.Fatal("expected unknown field rejection")
	}
}
