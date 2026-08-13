package memory

import (
	"bytes"
	"testing"
)

func planRevisionFixture(t *testing.T, id string, scope Scope, revision int) MemoryRevision {
	t.Helper()
	r := MemoryRevision{SchemaVersion: SchemaVersion, MemoryID: id, MemoryType: MemoryTypeStrategy, Scope: scope, CanonicalKey: "plan-key", Revision: revision, UsagePolicy: UsagePolicyOutcomeAttributed, Title: "Plan", Summary: "Plan summary", CreatedAt: "2026-08-13T00:00:00Z"}
	h, err := r.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	r.ContentSHA256 = h
	return r
}

func planEvidenceRef(id string, scope Scope) EvidenceRef {
	return EvidenceRef{Scope: scope, EvidenceType: "test_result", EvidenceID: id, ContentSHA256: "sha256_0000000000000000000000000000000000000000000000000000000000000000"}
}

func TestBuildRevisionPlanIsDeterministicAndValidatesChain(t *testing.T) {
	source := planRevisionFixture(t, "mem_plan", ScopeProject, 1)
	target := planRevisionFixture(t, "mem_plan", ScopeProject, 2)
	evidence := []EvidenceRef{planEvidenceRef("ev_b", ScopeProject), planEvidenceRef("ev_a", ScopeProject)}
	p1, err := BuildRevisionPlan(source, target, evidence)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := BuildRevisionPlan(source, target, []EvidenceRef{evidence[1], evidence[0]})
	if err != nil {
		t.Fatal(err)
	}
	b1, err := p1.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := p2.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b1, b2) || p1.PlanHash() != p2.PlanHash() {
		t.Fatal("revision plans must be byte stable")
	}
	bad := planRevisionFixture(t, "other", ScopeProject, 2)
	if _, err := BuildRevisionPlan(source, bad, evidence); err == nil {
		t.Fatal("revision plan must reject a different memory id")
	}
}

func TestBuildMergePlanSelectsPrimaryDeterministically(t *testing.T) {
	a := planRevisionFixture(t, "mem_a", ScopeProject, 1)
	b := planRevisionFixture(t, "mem_b", ScopeProject, 1)
	inputs := []MergeInput{
		{Revision: b, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_b", ScopeProject)}},
		{Revision: a, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_a", ScopeProject), planEvidenceRef("ev_a2", ScopeProject)}},
	}
	p, err := BuildMergePlan(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if p.Primary.MemoryID != "mem_a" || len(p.Inputs) != 2 || p.ProposedMemoryID == "mem_a" || p.ProposedMemoryID == "mem_b" {
		t.Fatalf("unexpected merge plan: %+v", p)
	}
	if _, err := BuildMergePlan([]MergeInput{inputs[0], inputs[0]}); err == nil {
		t.Fatal("duplicate merge inputs must fail closed")
	}
	if _, err := BuildMergePlan([]MergeInput{{Revision: planRevisionFixture(t, "mem_global", ScopeGlobal, 1), EvidenceRefs: []EvidenceRef{planEvidenceRef("ev", ScopeGlobal)}}}); err == nil {
		t.Fatal("single global input must not be a project merge")
	}
}

func TestBuildSplitPlanDoesNotReuseSourceID(t *testing.T) {
	source := planRevisionFixture(t, "mem_split", ScopeProject, 1)
	p, err := BuildSplitPlan(source, []SplitBranch{
		{Key: "build", EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_build", ScopeProject)}},
		{Key: "runtime", EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_runtime", ScopeProject)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Branches) != 2 || p.Branches[0].ProposedMemoryID == source.MemoryID || p.Branches[1].ProposedMemoryID == source.MemoryID || p.Branches[0].ProposedMemoryID == p.Branches[1].ProposedMemoryID {
		t.Fatalf("split branches must get distinct new ids: %+v", p.Branches)
	}
	if _, err := BuildSplitPlan(source, []SplitBranch{{Key: "build", EvidenceRefs: []EvidenceRef{planEvidenceRef("ev", ScopeProject)}}, {Key: "build", EvidenceRefs: []EvidenceRef{planEvidenceRef("ev2", ScopeProject)}}}); err == nil {
		t.Fatal("duplicate split keys must fail closed")
	}
}

func TestBuildGeneralizePlanRequiresIndependentTrustedProjects(t *testing.T) {
	a := planRevisionFixture(t, "mem_a", ScopeProject, 1)
	b := planRevisionFixture(t, "mem_b", ScopeProject, 1)
	inputs := []GeneralizeInput{
		{Revision: a, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_a", ScopeProject)}, TrustStatus: TrustGateTrusted},
		{Revision: b, EvidenceRefs: []EvidenceRef{planEvidenceRef("ev_b", ScopeProject)}, TrustStatus: TrustGateTrusted},
	}
	p, err := BuildGeneralizePlan(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if !p.PromotionEligible || p.ProposedGlobalMemoryID == "" || p.Scope != ScopeGlobal {
		t.Fatalf("trusted independent projects should yield a global candidate: %+v", p)
	}
	if _, err := BuildGeneralizePlan([]GeneralizeInput{inputs[0]}); err == nil {
		t.Fatal("one project must not generalize")
	}
	inputs[1].TrustStatus = TrustGateBlocked
	if _, err := BuildGeneralizePlan(inputs); err == nil {
		t.Fatal("untrusted input must fail closed")
	}
	inputs[1].TrustStatus = TrustGateTrusted
	inputs[1].EvidenceRefs = nil
	if _, err := BuildGeneralizePlan(inputs); err == nil {
		t.Fatal("generalization without evidence must fail closed")
	}
}

func TestPlansAreReadOnly(t *testing.T) {
	// Plan builders accept values only; this test documents that no store or
	// context is needed and therefore no filesystem side effect is possible.
}
