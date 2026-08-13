package memory

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPublishPromotionGenerationCommitsGlobalOKF(t *testing.T) {
	ctx := context.Background()
	global, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	sources := make([]PromotionCandidateSource, 0, 3)
	refs := make([]MemoryRef, 0, 3)
	families := []string{testHash, "sha256_" + strings.Repeat("1", 64), "sha256_" + strings.Repeat("2", 64)}
	for i, family := range families {
		s, err := OpenProject(tempRoot(t), Options{})
		if err != nil {
			t.Fatal(err)
		}
		r := planRevisionFixture(t, "promotion_source_"+string(rune('a'+i)), ScopeProject, 1)
		if _, err := s.Put(ctx, r); err != nil {
			t.Fatal(err)
		}
		ref := memoryRefFromRevision(r)
		refs = append(refs, ref)
		sources = append(sources, PromotionCandidateSource{Ref: ref, Store: s, FamilyFingerprint: family})
	}
	candidate := GlobalPromotionCandidate{SchemaVersion: SchemaVersion, CandidateID: "promotion_generation_candidate", Status: promotionCandidateEligible, UsagePolicy: UsagePolicyOutcomeAttributed, SourceMemoryRefs: refs, SourceProjectFamilyFingerprints: families, OutcomeRefs: []string{"outcome_promotion_generation"}, CriticJudgmentRefs: []JudgmentRef{}, EvidenceRefs: []EvidenceRef{}, ProposedAppliesWhen: []ApplicabilityCondition{}, ProposedDoesNotApplyWhen: []ApplicabilityCondition{}}
	h, err := candidate.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	candidate.ContentSHA256 = h
	if err := candidate.Validate(); err != nil {
		t.Fatalf("candidate fixture invalid: %v", err)
	}
	target := planRevisionFixture(t, "global_promotion_target", ScopeGlobal, 1)
	if target.UsagePolicy != candidate.UsagePolicy {
		t.Fatal("fixture policy mismatch")
	}
	rel := make([]MemoryRelation, 0, len(refs))
	for _, ref := range refs {
		rel = append(rel, MemoryRelation{Predicate: "generalized_from", Target: ref})
	}
	target.Relations = rel
	target.ContentSHA256, err = target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	ev := validEvidenceGeneration()
	ev.MemoryID, ev.EvidenceSetSHA256 = target.MemoryID, ""
	ev.Revision = target.Revision
	ev.EvidenceSetSHA256, err = ev.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := global.Put(ctx, ev); err != nil {
		t.Fatal(err)
	}
	policy := policyOf(PolicyTypeIndex)
	if _, err := global.Put(ctx, policy); err != nil {
		t.Fatal(err)
	}
	if _, err := global.Put(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyPromotionCandidate(ctx, PromotionCandidateApplyRequest{Candidate: candidate, Sources: sources, Target: target, Global: global}); err != nil {
		t.Fatal(err)
	}
	result, err := PublishPromotionGeneration(ctx, PromotionGenerationRequest{Candidate: candidate, Sources: sources, Target: target, Global: global, Compile: OKFCompileRequest{IndexPolicyRef: policyRefOf(policy), Revisions: []MemoryRevisionRef{{MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256}}, Evidence: []MemoryEvidenceRef{{MemoryID: ev.MemoryID, Revision: ev.Revision, EvidenceGeneration: ev.EvidenceGeneration, EvidenceSetSHA256: ev.EvidenceSetSHA256}}}, EvaluationTime: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC), IdempotencyKey: "promotion_generation_1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != CommitCommitted {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if _, err := global.Get(ctx, FactKindGenerationInputManifest, result.GenerationID); err != nil {
		t.Fatal(err)
	}
	cur, err := NewGenerationStore(global).(*generationStore).readCurrent(ctx)
	if err != nil || cur.GenerationID != result.GenerationID {
		t.Fatalf("CURRENT mismatch: %+v %v", cur, err)
	}
	replay, err := PublishPromotionGeneration(ctx, PromotionGenerationRequest{Candidate: candidate, Sources: sources, Target: target, Global: global, Compile: OKFCompileRequest{IndexPolicyRef: policyRefOf(policy), Revisions: []MemoryRevisionRef{{MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256}}, Evidence: []MemoryEvidenceRef{{MemoryID: ev.MemoryID, Revision: ev.Revision, EvidenceGeneration: ev.EvidenceGeneration, EvidenceSetSHA256: ev.EvidenceSetSHA256}}}, EvaluationTime: time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC), IdempotencyKey: "promotion_generation_1"})
	if err != nil || replay.Status != CommitAlreadyCommitted || replay.GenerationID != result.GenerationID {
		t.Fatalf("replay must return the durable commit: %+v %v", replay, err)
	}
	changedTime := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	changed, err := PublishPromotionGeneration(ctx, PromotionGenerationRequest{Candidate: candidate, Sources: sources, Target: target, Global: global, Compile: OKFCompileRequest{IndexPolicyRef: policyRefOf(policy), Revisions: []MemoryRevisionRef{{MemoryID: target.MemoryID, Revision: target.Revision, ContentSHA256: target.ContentSHA256}}, Evidence: []MemoryEvidenceRef{{MemoryID: ev.MemoryID, Revision: ev.Revision, EvidenceGeneration: ev.EvidenceGeneration, EvidenceSetSHA256: ev.EvidenceSetSHA256}}}, EvaluationTime: changedTime, IdempotencyKey: "promotion_generation_1"})
	if ErrorCode(err) != CodeGenerationIdempotency || changed.GenerationID != "" {
		t.Fatalf("changed promotion request with same key must fail closed before reuse: %+v %v", changed, err)
	}
}

func TestPublishPromotionGenerationRequiresExplicitTime(t *testing.T) {
	global, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PublishPromotionGeneration(context.Background(), PromotionGenerationRequest{Global: global}); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero evaluation time must fail deterministically: %v", err)
	}
}
