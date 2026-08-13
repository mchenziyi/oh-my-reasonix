package memory

import (
	"context"
	"testing"
)

func TestApplyMergePlanCreatesNewImmutableRevision(t *testing.T) {
	ctx := context.Background()
	store := openProject(t, tempRoot(t), Options{})
	first := validRevision()
	second := first
	second.MemoryID = "mem_01K7A9X3"
	second.CanonicalKey = "verify-before-upgrade-retry-alt"
	second.ContentSHA256 = ""
	hash, err := second.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	second.ContentSHA256 = hash
	for _, rev := range []MemoryRevision{first, second} {
		if _, err := store.Put(ctx, rev); err != nil {
			t.Fatal(err)
		}
	}
	ev := validEvidenceGeneration()
	if _, err := store.Put(ctx, ev); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildMergePlan([]MergeInput{{Revision: first, EvidenceRefs: ev.EvidenceRefs}, {Revision: second, EvidenceRefs: ev.EvidenceRefs}})
	if err != nil {
		t.Fatal(err)
	}
	target := first
	target.MemoryID = plan.ProposedMemoryID
	target.CanonicalKey = "merged-memory"
	target.Revision = 1
	target.ContentSHA256 = ""
	targetHash, err := target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	target.ContentSHA256 = targetHash
	result, err := ApplyMergePlan(ctx, store, plan, target)
	if err != nil || result.Status != WriteCreated {
		t.Fatalf("merge apply failed: result=%+v err=%v", result, err)
	}
	replay, err := ApplyMergePlan(ctx, store, plan, target)
	if err != nil || replay.Status != WriteNoop {
		t.Fatalf("merge replay must be noop: result=%+v err=%v", replay, err)
	}
	if _, err := store.Get(ctx, FactKindMemoryRevision, revisionKey(memoryRefFromRevision(first))); err != nil {
		t.Fatalf("source revision must remain: %v", err)
	}
}

func TestApplyMergePlanRejectsMissingEvidenceBeforeWrite(t *testing.T) {
	ctx := context.Background()
	store := openProject(t, tempRoot(t), Options{})
	first := validRevision()
	second := first
	second.MemoryID = "mem_01K7A9X3"
	second.CanonicalKey = "verify-before-upgrade-retry-alt"
	second.ContentSHA256 = ""
	hash, err := second.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	second.ContentSHA256 = hash
	for _, rev := range []MemoryRevision{first, second} {
		if _, err := store.Put(ctx, rev); err != nil {
			t.Fatal(err)
		}
	}
	missing := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_missing", ContentSHA256: testHash}
	plan, err := BuildMergePlan([]MergeInput{{Revision: first, EvidenceRefs: []EvidenceRef{missing}}, {Revision: second, EvidenceRefs: []EvidenceRef{missing}}})
	if err != nil {
		t.Fatal(err)
	}
	target := first
	target.MemoryID = plan.ProposedMemoryID
	target.Revision = 1
	target.ContentSHA256 = ""
	hash, err = target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	target.ContentSHA256 = hash
	if _, err := ApplyMergePlan(ctx, store, plan, target); ErrorCode(err) != CodeNotFound {
		t.Fatalf("missing evidence must fail closed before write, got %v", err)
	}
	if _, err := store.Get(ctx, FactKindMemoryRevision, plan.ProposedMemoryID+"/1"); ErrorCode(err) != CodeNotFound {
		t.Fatalf("failed merge must not write target, got %v", err)
	}
}
