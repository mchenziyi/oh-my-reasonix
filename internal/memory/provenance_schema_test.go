package memory

import (
	"context"
	"encoding/json"
	"testing"
)

// MEM-02C 8.1: Provenance schema validation matrix. Written before the six
// fields existed (compile failure), then completed per plan.

func enrichedEvidence(ev MemoryEvidenceGeneration) MemoryEvidenceGeneration {
	true_, false_ := true, false
	ev.EvidenceOrigin = "runtime"
	ev.AcquisitionMethod = "tool_observed"
	ev.VerificationStatus = "verified"
	ev.ProvenanceRefs = []EvidenceRef{ev.EvidenceRefs[0]}
	ev.ContainsInstructionalContent = &true_
	ev.ContainsSensitiveContent = &false_
	return fillEvidenceHash(ev)
}

func TestProvenanceValidEnumerations(t *testing.T) {
	for _, origin := range []string{"runtime", "user", "official", "project", "external"} {
		ev := enrichedEvidence(validEvidenceGeneration())
		ev.EvidenceOrigin = origin
		ev = fillEvidenceHash(ev)
		if err := ev.Validate(); err != nil {
			t.Errorf("origin %q should be valid: %v", origin, err)
		}
	}
	for _, m := range []string{"direct", "tool_observed", "model_extracted", "imported"} {
		ev := enrichedEvidence(validEvidenceGeneration())
		ev.AcquisitionMethod = m
		ev = fillEvidenceHash(ev)
		if err := ev.Validate(); err != nil {
			t.Errorf("acquisition %q should be valid: %v", m, err)
		}
	}
	for _, v := range []string{"verified", "confirmed", "inferred", "unverified"} {
		ev := enrichedEvidence(validEvidenceGeneration())
		ev.VerificationStatus = v
		ev = fillEvidenceHash(ev)
		if err := ev.Validate(); err != nil {
			t.Errorf("verification %q should be valid: %v", v, err)
		}
	}
}

func TestProvenanceRejectsUnknownValues(t *testing.T) {
	for _, v := range []string{"self_hosted", "auto", ""} {
		ev := enrichedEvidence(validEvidenceGeneration())
		ev.EvidenceOrigin = v
		if err := ev.Validate(); err == nil {
			t.Errorf("origin %q must be rejected", v)
		}
	}
	for _, v := range []string{"llm_generated", ""} {
		ev := enrichedEvidence(validEvidenceGeneration())
		ev.AcquisitionMethod = v
		if err := ev.Validate(); err == nil {
			t.Errorf("acquisition %q must be rejected", v)
		}
	}
	for _, v := range []string{"partially_verified", ""} {
		ev := enrichedEvidence(validEvidenceGeneration())
		ev.VerificationStatus = v
		if err := ev.Validate(); err == nil {
			t.Errorf("verification %q must be rejected", v)
		}
	}
}

func TestProvenancePartialFieldsRejected(t *testing.T) {
	// Each single field present without the full six must fail closed.
	ev := validEvidenceGeneration()
	ev.EvidenceOrigin = "runtime"
	if err := ev.Validate(); err == nil {
		t.Error("partial provenance (origin only) must be rejected")
	}
	ev2 := validEvidenceGeneration()
	b := true
	ev2.ContainsInstructionalContent = &b
	if err := ev2.Validate(); err == nil {
		t.Error("partial provenance (boolean only) must be rejected")
	}
	ev3 := validEvidenceGeneration()
	ev3.ProvenanceRefs = []EvidenceRef{}
	if err := ev3.Validate(); err == nil {
		t.Error("partial provenance (empty refs only) must be rejected")
	}
}

func TestProvenanceNilVsEmptyRefs(t *testing.T) {
	// direct with explicit empty refs is valid.
	direct := enrichedEvidence(validEvidenceGeneration())
	direct.AcquisitionMethod = "direct"
	direct.ProvenanceRefs = []EvidenceRef{}
	direct = fillEvidenceHash(direct)
	if err := direct.Validate(); err != nil {
		t.Errorf("direct with explicit empty refs must be valid: %v", err)
	}
	// Non-direct with empty refs is invalid.
	nonDirect := enrichedEvidence(validEvidenceGeneration())
	nonDirect.ProvenanceRefs = []EvidenceRef{}
	nonDirect = fillEvidenceHash(nonDirect)
	if err := nonDirect.Validate(); err == nil {
		t.Error("non-direct acquisition with empty provenance must be rejected")
	}
}

func TestProvenanceRefsClosedOverEvidenceRefs(t *testing.T) {
	// Refs must be exact members of evidence_refs: wrong id/scope/hash rejected.
	base := validEvidenceGeneration()
	bad := []EvidenceRef{
		{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_999", ContentSHA256: testHash},
		{Scope: ScopeGlobal, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash},
		{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash2},
	}
	for i, ref := range bad {
		ev := enrichedEvidence(base)
		ev.ProvenanceRefs = []EvidenceRef{ref}
		if err := ev.Validate(); err == nil {
			t.Errorf("bad provenance ref #%d must be rejected", i)
		}
	}
	// Duplicate refs rejected.
	dup := enrichedEvidence(base)
	dup.ProvenanceRefs = []EvidenceRef{base.EvidenceRefs[0], base.EvidenceRefs[0]}
	if err := dup.Validate(); err == nil {
		t.Error("duplicate provenance refs must be rejected")
	}
	// Too many refs rejected.
	tooMany := enrichedEvidence(base)
	for i := 0; i <= maxRefs; i++ {
		tooMany.ProvenanceRefs = append(tooMany.ProvenanceRefs, base.EvidenceRefs[0])
	}
	if err := tooMany.Validate(); err == nil {
		t.Error("provenance_refs over maxRefs must be rejected")
	}
}

func TestProvenanceBooleanExplicitPresence(t *testing.T) {
	// Explicit false is a real boolean value in the canonical form; absent
	// (Legacy) means the field does not exist at all. The JSON must carry
	// the explicit false key, and true/false must hash differently inside
	// the Enriched form.
	legacy := validEvidenceGeneration()
	ljson, err := legacy.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	var lm map[string]any
	if err := json.Unmarshal(ljson, &lm); err != nil {
		t.Fatal(err)
	}
	if _, ok := lm["contains_sensitive_content"]; ok {
		t.Error("legacy JSON must not carry the boolean keys")
	}
	ejson, err := enrichedEvidence(validEvidenceGeneration()).EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	var em map[string]any
	if err := json.Unmarshal(ejson, &em); err != nil {
		t.Fatal(err)
	}
	sens, ok := em["contains_sensitive_content"]
	if !ok {
		t.Errorf("enriched JSON must carry explicit false, got %s", ejson)
	}
	if sens != false {
		t.Errorf("enriched contains_sensitive_content must be explicit false, got %v", sens)
	}
	hFalse, err := enrichedEvidence(validEvidenceGeneration()).ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	tr := true
	withTrue := enrichedEvidence(validEvidenceGeneration())
	withTrue.ContainsSensitiveContent = &tr
	hTrue, err := withTrue.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if hFalse == hTrue {
		t.Error("explicit true and explicit false must hash differently")
	}
}

func TestProvenanceAnyFieldChangeAltersHash(t *testing.T) {
	// Evidence generation with two evidence members so a provenance set can
	// change membership without becoming invalid.
	base := enrichedEvidence(validEvidenceGeneration())
	second := base.EvidenceRefs[0]
	second.EvidenceID = "episode_002"
	base.EvidenceRefs = append(base.EvidenceRefs, second)
	base = fillEvidenceHash(base)
	h0, err := base.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	muts := []func(MemoryEvidenceGeneration) MemoryEvidenceGeneration{
		func(e MemoryEvidenceGeneration) MemoryEvidenceGeneration { e.EvidenceOrigin = "official"; return e },
		func(e MemoryEvidenceGeneration) MemoryEvidenceGeneration { e.AcquisitionMethod = "imported"; return e },
		func(e MemoryEvidenceGeneration) MemoryEvidenceGeneration { e.VerificationStatus = "inferred"; return e },
		func(e MemoryEvidenceGeneration) MemoryEvidenceGeneration {
			e.ProvenanceRefs = []EvidenceRef{second}
			return e
		},
		func(e MemoryEvidenceGeneration) MemoryEvidenceGeneration {
			b := false
			e.ContainsInstructionalContent = &b
			return e
		},
	}
	for i, mut := range muts {
		h, err := mut(base).ContentHash()
		if err != nil {
			t.Fatal(err)
		}
		if h == h0 {
			t.Errorf("mutation #%d must change the hash", i)
		}
	}
}

func TestProvenanceEnrichedRoundTripAndNoop(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ev := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	_, key, kerr := factKey(ev)
	if kerr != nil {
		t.Fatal(kerr)
	}
	raw, err := s.Get(context.Background(), FactKindMemoryEvidenceGeneration, key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeStrict[MemoryEvidenceGeneration](raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.EvidenceOrigin != "runtime" || got.AcquisitionMethod != "tool_observed" ||
		got.VerificationStatus != "verified" || len(got.ProvenanceRefs) != 1 {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if got.ContainsInstructionalContent == nil || !*got.ContainsInstructionalContent {
		t.Error("instructional boolean lost in round trip")
	}
	if got.ContainsSensitiveContent == nil || *got.ContainsSensitiveContent {
		t.Error("sensitive boolean lost in round trip")
	}
	// NOOP on identical re-put.
	res, err := s.Put(context.Background(), got)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != WriteNoop {
		t.Errorf("identical enriched re-put should be noop, got %s", res.Status)
	}
}

func TestProvenanceLegacyEnrichedIdentityConflict(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	legacy := validEvidenceGeneration()
	if _, err := s.Put(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	enriched := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), enriched); err == nil {
		t.Error("same identity with different provenance must conflict, not overwrite")
	}
}

// TestProvenancePartialCanonMapFailsClosed: calling the canonical form on a
// partial (invalid) provenance set must return an error, never panic on a
// nil boolean dereference.
func TestProvenancePartialCanonMapFailsClosed(t *testing.T) {
	ev := validEvidenceGeneration()
	b := true
	ev.EvidenceOrigin = "runtime"
	ev.ContainsInstructionalContent = &b
	if _, err := ev.CanonicalBytes(); err == nil {
		t.Fatal("partial provenance canonical bytes must fail closed")
	}
	if _, err := ev.ContentHash(); err == nil {
		t.Fatal("partial provenance hash must fail closed")
	}
}
