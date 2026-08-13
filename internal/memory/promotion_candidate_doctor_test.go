package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestConsistencyDoctorAuditsPromotionCandidates(t *testing.T) {
	s, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	c := candidateFixture(t)
	if _, err := s.Put(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	clean, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeGlobal})
	if err != nil || !clean.Healthy {
		t.Fatalf("valid candidate should be healthy: %+v %v", clean, err)
	}
	path := filepath.Join(s.root, "facts", string(FactKindPromotionCandidate), c.CandidateID+".json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"candidate_id":"bad"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeGlobal})
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy || !hasFinding(report, findingCorruptFact) {
		t.Fatalf("doctor must report corrupt candidate: %+v", report.Findings)
	}
}
