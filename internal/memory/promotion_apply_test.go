package memory

import (
	"context"
	"testing"
)

func TestApplyPromotionPlanWritesOnlyGlobalTarget(t *testing.T) {
	ctx := context.Background()
	project := openProject(t, tempRoot(t), Options{})
	global, err := OpenGlobal(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	policy := promotionPolicyFixture(t)
	policyHash, _ := policy.ContentHash()
	a := promotionInputFixture(t, "mem_promote_a", "sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "root_promote_a")
	b := promotionInputFixture(t, "mem_promote_b", "sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "root_promote_b")
	for _, input := range []PromotionInput{a, b} {
		if _, err := project.Put(ctx, input.Revision); err != nil {
			t.Fatal(err)
		}
		ev := validEvidenceGeneration()
		ev.MemoryID = input.Revision.MemoryID
		ev.EvidenceRefs = input.EvidenceRefs
		ev.EvidenceSetSHA256 = ""
		h, err := ev.ContentHash()
		if err != nil {
			t.Fatal(err)
		}
		ev.EvidenceSetSHA256 = h
		if _, err := project.Put(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := BuildPromotionPlan(PromotionRequest{Policy: policy, PolicyRef: PolicyRef{PolicyID: policy.PolicyID, PolicyType: policy.PolicyType, ContentSHA256: policyHash}, Inputs: []PromotionInput{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	target := a.Revision
	target.Scope = ScopeGlobal
	target.MemoryID = plan.ProposedGlobalMemoryID
	target.CanonicalKey = "promoted-memory"
	target.ContentSHA256 = ""
	targetHash, err := target.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	target.ContentSHA256 = targetHash
	result, err := ApplyPromotionPlan(ctx, project, global, plan, policy, target)
	if err != nil || result.Status != WriteCreated {
		t.Fatalf("promotion apply failed: result=%+v err=%v", result, err)
	}
	if _, err := project.Get(ctx, FactKindMemoryRevision, revisionKey(aRevisionRef(a))); err != nil {
		t.Fatalf("project source must remain: %v", err)
	}
	if _, err := global.Get(ctx, FactKindMemoryRevision, plan.ProposedGlobalMemoryID+"/1"); err != nil {
		t.Fatalf("global target missing: %v", err)
	}
}

func aRevisionRef(input PromotionInput) MemoryRef { return memoryRefFromRevision(input.Revision) }
