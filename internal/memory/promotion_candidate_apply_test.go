package memory

import (
	"context"
	"testing"
)

func TestApplyPromotionCandidateBindsExplicitProjectStores(t *testing.T) {
	ctx := context.Background()
	stores := []*FactStore{openProject(t, tempRoot(t), Options{}), openProject(t, tempRoot(t), Options{}), openProject(t, tempRoot(t), Options{})}
	refs := make([]MemoryRef, 0, len(stores))
	families := []string{
		"sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"sha256_cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
	}
	bindings := make([]PromotionCandidateSource, 0, len(stores))
	for i, store := range stores {
		rev := planRevisionFixture(t, "candidate_apply_memory_"+string(rune('a'+i)), ScopeProject, 1)
		if _, err := store.Put(ctx, rev); err != nil {
			t.Fatal(err)
		}
		ref := memoryRefFromRevision(rev)
		refs = append(refs, ref)
		bindings = append(bindings, PromotionCandidateSource{Ref: ref, Store: store, FamilyFingerprint: families[i]})
	}
	candidate := GlobalPromotionCandidate{
		SchemaVersion: SchemaVersion, CandidateID: "promotion_apply_candidate", Status: promotionCandidateEligible,
		UsagePolicy: UsagePolicyOutcomeAttributed, SourceMemoryRefs: refs,
		SourceProjectFamilyFingerprints: families, OutcomeRefs: []string{"outcome_candidate_apply"},
		EvidenceRefs: []EvidenceRef{}, CriticJudgmentRefs: []JudgmentRef{}, ProposedAppliesWhen: []ApplicabilityCondition{}, ProposedDoesNotApplyWhen: []ApplicabilityCondition{},
	}
	h, err := candidate.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	candidate.ContentSHA256 = h
	target := planRevisionFixture(t, "global_candidate_apply", ScopeGlobal, 1)
	target.Relations = make([]MemoryRelation, 0, len(refs))
	for _, ref := range refs {
		target.Relations = append(target.Relations, MemoryRelation{Predicate: "generalized_from", Target: ref})
	}
	target.ContentSHA256 = ""
	target.ContentSHA256, err = target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	global := mustOpenStore(t, tempRoot(t), StoreScopeGlobal)
	result, err := ApplyPromotionCandidate(ctx, PromotionCandidateApplyRequest{Candidate: candidate, Sources: bindings, Target: target, Global: global})
	if err != nil || result.Status != WriteCreated {
		t.Fatalf("candidate apply failed: %+v %v", result, err)
	}
	replay, err := ApplyPromotionCandidate(ctx, PromotionCandidateApplyRequest{Candidate: candidate, Sources: bindings, Target: target, Global: global})
	if err != nil || replay.Status != WriteNoop {
		t.Fatalf("candidate replay must noop: %+v %v", replay, err)
	}
}

func TestApplyPromotionCandidateRejectsUnboundSourceBeforeWrite(t *testing.T) {
	store := openProject(t, tempRoot(t), Options{})
	rev := planRevisionFixture(t, "candidate_unbound", ScopeProject, 1)
	if _, err := store.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	ref := memoryRefFromRevision(rev)
	candidate := GlobalPromotionCandidate{SchemaVersion: SchemaVersion, CandidateID: "promotion_unbound_candidate", Status: promotionCandidateCollecting, UsagePolicy: UsagePolicyOutcomeAttributed, SourceMemoryRefs: []MemoryRef{ref, ref}, SourceProjectFamilyFingerprints: []string{"sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, OutcomeRefs: []string{"outcome_unbound"}, EvidenceRefs: []EvidenceRef{}, CriticJudgmentRefs: []JudgmentRef{}, ProposedAppliesWhen: []ApplicabilityCondition{}, ProposedDoesNotApplyWhen: []ApplicabilityCondition{}}
	candidate.ContentSHA256, _ = candidate.ContentHash()
	global := mustOpenStore(t, tempRoot(t), StoreScopeGlobal)
	if _, err := ApplyPromotionCandidate(context.Background(), PromotionCandidateApplyRequest{Candidate: candidate, Sources: []PromotionCandidateSource{{Ref: ref, Store: store, FamilyFingerprint: candidate.SourceProjectFamilyFingerprints[0]}}, Target: planRevisionFixture(t, "global_unbound", ScopeGlobal, 1), Global: global}); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("unbound candidate must fail closed, got %v", err)
	}
}
