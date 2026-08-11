package memory

import (
	"testing"
)

// MEM-02C 8.1-2: the Legacy (six provenance fields all absent) canonical
// bytes and content hash are frozen from the pre-MEM-02C implementation and
// must never change, so historical generations keep rebuilding identically.
func TestLegacyEvidenceGoldenFrozen(t *testing.T) {
	ev := validEvidenceGeneration() // Legacy: no provenance fields set.
	got, err := ev.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	wantCanon := `{"created_at":"2026-08-07T00:00:00Z","evidence_generation":3,"evidence_refs":[{"content_sha256":"sha256_0000000000000000000000000000000000000000000000000000000000000000","evidence_id":"episode_001","evidence_type":"episode","scope":"project"}],"memory_id":"mem_01K7A9X2","previous_evidence_generation":null,"revision":2,"root_task_refs":[],"schema_version":1,"transaction_id":"tx_01K"}`
	if string(got) != wantCanon {
		t.Errorf("legacy canonical bytes changed:\n got %s\nwant %s", got, wantCanon)
	}
	h, err := ev.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if h != "sha256_25a20de3baac5268b141cd102b298d43fc28a1337d509c16437bcd889518a6a2" {
		t.Errorf("legacy content hash changed: got %s", h)
	}
}
