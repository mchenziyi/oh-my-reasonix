package memory

import (
	"strings"
	"testing"
)

func TestCanonicalEncodingDeterministic(t *testing.T) {
	r := validRevision()
	b1, err := r.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := r.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Error("CanonicalBytes must be deterministic across calls")
	}
	e1, err := r.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	e2, err := r.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if string(e1) != string(e2) {
		t.Error("EncodeCanonical must be deterministic across calls")
	}
	if string(b1) == string(e1) {
		t.Error("EncodeCanonical should include the hash field")
	}
}

func TestContentHashExcludesHashField(t *testing.T) {
	r := validRevision()
	h1, err := r.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h1, "sha256_") || len(h1) != len("sha256_")+64 {
		t.Fatalf("ContentHash format wrong: %q", h1)
	}
	// Changing only the stored hash field must not change the content hash
	// (hash field is excluded from canonical bytes).
	r2 := r
	r2.ContentSHA256 = "sha256_" + strings.Repeat("f", 64)
	h2, err := r2.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Error("ContentHash must exclude the hash field itself")
	}
}

func TestProtectedFieldChangeChangesHash(t *testing.T) {
	r := validRevision()
	h1, _ := r.ContentHash()
	modified := r
	modified.Title = "Different Title"
	h2, _ := modified.ContentHash()
	if h1 == h2 {
		t.Error("changing a protected field must change the content hash")
	}
	// usage_policy is protected (immutable after creation).
	policyChanged := r
	policyChanged.UsagePolicy = UsagePolicyEvidenceValidated
	h3, _ := policyChanged.ContentHash()
	if h1 == h3 {
		t.Error("changing usage_policy must change the content hash")
	}
}

func TestEvidenceRefsOrderAndDuplicatesHash(t *testing.T) {
	base := MemoryEvidenceGeneration{
		SchemaVersion:      1,
		MemoryID:           "mem_01",
		Revision:           1,
		EvidenceGeneration: 1,
		TransactionID:      "tx_01",
		CreatedAt:          "2026-08-07T00:00:00Z",
	}
	refA := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_a", ContentSHA256: testHash}
	refB := EvidenceRef{Scope: ScopeGlobal, EvidenceType: "outcome", EvidenceID: "out_b", ContentSHA256: testHash}

	ordered := base
	ordered.EvidenceRefs = []EvidenceRef{refA, refB}

	shuffled := base
	shuffled.EvidenceRefs = []EvidenceRef{refB, refA, refB, refA}

	h1, err := ordered.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := shuffled.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Error("evidence_set hash must be independent of ref order and duplicates")
	}
}

func TestAliasesOrderAndDuplicatesHash(t *testing.T) {
	a := validRevision()
	a.Aliases = []string{"alpha", "beta"}
	ha, _ := a.ContentHash()

	b := validRevision()
	b.Aliases = []string{"beta", "alpha", "beta"}
	hb, _ := b.ContentHash()
	if ha != hb {
		t.Error("alias set hash must be independent of order and duplicates")
	}
}

func TestTimeNormalizedToUTCInCanonical(t *testing.T) {
	a := validRevision()
	b := validRevision()
	b.CreatedAt = "2026-08-07T08:00:00+08:00" // same instant as 00:00:00Z
	ha, _ := a.ContentHash()
	hb, _ := b.ContentHash()
	if ha != hb {
		t.Error("equivalent instants in different offsets must produce the same hash")
	}
}

func TestDecodeStrictRejectsUnknownFields(t *testing.T) {
	in := `{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z","extra_field":true}`
	if _, err := DecodeStrict[MemoryRevision]([]byte(in)); err == nil {
		t.Error("unknown field must be rejected")
	}
}

func TestDecodeStrictRejectsWrongTypes(t *testing.T) {
	in := `{"schema_version":"one","memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"}`
	if _, err := DecodeStrict[MemoryRevision]([]byte(in)); err == nil {
		t.Error("wrong field type must be rejected")
	}
}

func TestDecodeStrictRejectsMissingFields(t *testing.T) {
	in := `{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"}`
	if _, err := DecodeStrict[MemoryRevision]([]byte(in)); err == nil {
		t.Error("missing field must be rejected")
	}
}

func TestDecodeStrictRejectsFreeTextConditions(t *testing.T) {
	in := `{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":["use only when the driver is modernc"],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"}`
	if _, err := DecodeStrict[MemoryRevision]([]byte(in)); err == nil {
		t.Error("free-text machine condition must be rejected")
	}
}

func TestDecodeStrictRejectsTrailingData(t *testing.T) {
	in := `{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"} {"x":1}`
	if _, err := DecodeStrict[MemoryRevision]([]byte(in)); err == nil {
		t.Error("trailing JSON data must be rejected")
	}
}

func TestDecodeStrictRoundTrip(t *testing.T) {
	r := validRevision()
	encoded, err := r.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeStrict[MemoryRevision](encoded)
	if err != nil {
		t.Fatalf("round trip failed: %v", err)
	}
	hd, _ := decoded.ContentHash()
	if hd != r.ContentSHA256 {
		t.Error("round-trip hash mismatch")
	}
}

func TestDecodeStrictRejectsHashMismatch(t *testing.T) {
	in := `{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z"}`
	if _, err := DecodeStrict[MemoryRevision]([]byte(in)); err == nil {
		t.Error("hash mismatch must be rejected")
	}
}

func TestAllFactsImplementInterface(t *testing.T) {
	var _ Fact = MemoryRevision{}
	var _ Fact = MemoryEvidenceGeneration{}
	var _ Fact = JudgmentFact{}
	var _ Fact = PolicyFact{}
	var _ Fact = GenerationInputManifest{}
	var _ Fact = GovernanceEvent{}
}

func TestManifestHashCoversOutputHash(t *testing.T) {
	m := validManifest()
	h1, _ := m.ContentHash()
	m2 := m
	m2.OutputSHA256 = "sha256_" + strings.Repeat("9", 64)
	h2, _ := m2.ContentHash()
	if h1 == h2 {
		t.Error("input_manifest_sha256 must cover output_sha256")
	}
}
