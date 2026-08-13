package memory

import (
	"bytes"
	"context"
	"testing"
)

func candidateFixture(t *testing.T) GlobalPromotionCandidate {
	t.Helper()
	a := planRevisionFixture(t, "mem_candidate_a", ScopeProject, 1)
	b := planRevisionFixture(t, "mem_candidate_b", ScopeProject, 1)
	c := GlobalPromotionCandidate{
		SchemaVersion: SchemaVersion, CandidateID: "promotion_01Kcandidate", Status: promotionCandidateCollecting,
		UsagePolicy: UsagePolicyOutcomeAttributed, SourceMemoryRefs: []MemoryRef{memoryRefFromRevision(a), memoryRefFromRevision(b)},
		SourceProjectFamilyFingerprints: []string{"sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		OutcomeRefs:                     []string{"outcome_candidate_a"}, CriticJudgmentRefs: []JudgmentRef{}, EvidenceRefs: []EvidenceRef{},
		ProposedAppliesWhen: []ApplicabilityCondition{}, ProposedDoesNotApplyWhen: []ApplicabilityCondition{},
	}
	h, err := c.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	c.ContentSHA256 = h
	return c
}

func TestPromotionCandidateCanonicalAndStore(t *testing.T) {
	c := candidateFixture(t)
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	a, err := c.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	c.SourceMemoryRefs[0], c.SourceMemoryRefs[1] = c.SourceMemoryRefs[1], c.SourceMemoryRefs[0]
	b, err := c.CanonicalBytes()
	if err != nil || !bytes.Equal(a, b) {
		t.Fatalf("candidate encoding must be order independent: %v", err)
	}
	s, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Put(context.Background(), c)
	if err != nil || result.Status != WriteCreated {
		t.Fatalf("candidate put failed: %+v %v", result, err)
	}
	replay, err := s.Put(context.Background(), c)
	if err != nil || replay.Status != WriteNoop {
		t.Fatalf("candidate replay must noop: %+v %v", replay, err)
	}
	if _, err := s.Get(context.Background(), FactKindPromotionCandidate, c.CandidateID); err != nil {
		t.Fatal(err)
	}
}

func TestPromotionCandidateRejectsPolicyBorrowingAndCrossScope(t *testing.T) {
	c := candidateFixture(t)
	c.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_candidate", ContentSHA256: testHash}}
	c.ContentSHA256 = ""
	if err := c.Validate(); err == nil {
		t.Fatal("outcome candidate must reject borrowed evidence")
	}
	c = candidateFixture(t)
	c.SourceMemoryRefs[0].Scope = ScopeGlobal
	c.ContentSHA256 = ""
	if err := c.Validate(); err == nil {
		t.Fatal("candidate must reject global source ref")
	}
}

func TestPromotionCandidateEligibleRequiresThreeFamilies(t *testing.T) {
	c := candidateFixture(t)
	c.Status = promotionCandidateEligible
	c.ContentSHA256 = ""
	if err := c.Validate(); err == nil {
		t.Fatal("eligible candidate must require three families")
	}
	c.SourceProjectFamilyFingerprints = append(c.SourceProjectFamilyFingerprints, "sha256_cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	c.ContentSHA256 = ""
	h, err := c.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	c.ContentSHA256 = h
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
