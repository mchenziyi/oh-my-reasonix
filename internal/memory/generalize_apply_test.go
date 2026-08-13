package memory

import (
	"context"
	"testing"
)

func TestApplyGeneralizePlanPublishesGlobalTargetAndRetainsSources(t *testing.T) {
	ctx := context.Background()
	project := openProject(t, tempRoot(t), Options{})
	global, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := planRevisionFixture(t, "mem_generalize_a", ScopeProject, 1)
	b := planRevisionFixture(t, "mem_generalize_b", ScopeProject, 1)
	for _, source := range []MemoryRevision{a, b} {
		if _, err := project.Put(ctx, source); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := BuildGeneralizePlan([]GeneralizeInput{
		{Revision: a, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_ga", ScopeProject)}, TrustStatus: TrustGateTrusted},
		{Revision: b, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_gb", ScopeProject)}, TrustStatus: TrustGateTrusted},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := planRevisionFixture(t, plan.ProposedGlobalMemoryID, ScopeGlobal, 1)
	target.Relations = []MemoryRelation{
		{Predicate: "generalized_from", Target: memoryRefFromRevision(a)},
		{Predicate: "generalized_from", Target: memoryRefFromRevision(b)},
	}
	target.ContentSHA256 = ""
	target.ContentSHA256, err = target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	result, err := ApplyGeneralizePlan(ctx, project, global, plan, target)
	if err != nil || result.Status != WriteCreated {
		t.Fatalf("apply failed: %+v %v", result, err)
	}
	replay, err := ApplyGeneralizePlan(ctx, project, global, plan, target)
	if err != nil || replay.Status != WriteNoop {
		t.Fatalf("replay must noop: %+v %v", replay, err)
	}
	if _, err := global.Get(ctx, FactKindMemoryRevision, revisionKey(memoryRefFromRevision(target))); err != nil {
		t.Fatal(err)
	}
	for _, source := range []MemoryRevision{a, b} {
		if _, err := project.Get(ctx, FactKindMemoryRevision, revisionKey(memoryRefFromRevision(source))); err != nil {
			t.Fatalf("source removed: %v", err)
		}
	}
}

func TestApplyGeneralizePlanRejectsMissingRelationBeforeWrite(t *testing.T) {
	ctx := context.Background()
	project := openProject(t, tempRoot(t), Options{})
	global, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := planRevisionFixture(t, "mem_generalize_c", ScopeProject, 1)
	b := planRevisionFixture(t, "mem_generalize_d", ScopeProject, 1)
	for _, source := range []MemoryRevision{a, b} {
		if _, err := project.Put(ctx, source); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := BuildGeneralizePlan([]GeneralizeInput{{Revision: a, TrustStatus: TrustGateTrusted, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_gc", ScopeProject)}}, {Revision: b, TrustStatus: TrustGateTrusted, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_gd", ScopeProject)}}})
	if err != nil {
		t.Fatal(err)
	}
	target := planRevisionFixture(t, plan.ProposedGlobalMemoryID, ScopeGlobal, 1)
	target.ContentSHA256 = ""
	target.ContentSHA256, err = target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyGeneralizePlan(ctx, project, global, plan, target); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("missing generalized_from must fail closed, got %v", err)
	}
	if _, err := global.Get(ctx, FactKindMemoryRevision, plan.ProposedGlobalMemoryID+"/1"); ErrorCode(err) != CodeNotFound {
		t.Fatalf("failed apply wrote target: %v", err)
	}
}
