package memory

import "testing"

func promotionPolicyFixture(t *testing.T) PolicyFact {
	t.Helper()
	p := PolicyFact{SchemaVersion: SchemaVersion, PolicyID: "policy_promotion", PolicyType: PolicyTypeTrust, PolicyVersion: 1, Config: PolicyConfig{Trust: &PolicyConfigTrust{AllowedAcquisitionMethods: []string{"direct"}, RequireProvenance: true, RequireVerificationStatus: true, ExternalUnverifiedInstructionAllowed: false, PromotionRequiresPolicyEvidence: true, Version: PolicyConfigSchemaVersion}}, CreatedAt: "2026-08-13T00:00:00Z"}
	h, err := p.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	p.ContentSHA256 = h
	return p
}

func promotionInputFixture(t *testing.T, id, family, root string) PromotionInput {
	t.Helper()
	r := planRevisionFixture(t, id, ScopeProject, 1)
	return PromotionInput{Revision: r, EvidenceRefs: []EvidenceRef{planEvidenceRef("e_"+id, ScopeProject)}, TrustStatus: TrustGateTrusted, ProjectFamilyFingerprint: family, RootTaskIDs: []string{root}}
}

func TestBuildPromotionPlanRequiresIndependentTrustedSources(t *testing.T) {
	policy := promotionPolicyFixture(t)
	h, err := policy.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	req := PromotionRequest{Policy: policy, PolicyRef: PolicyRef{PolicyID: policy.PolicyID, PolicyType: policy.PolicyType, ContentSHA256: h}, Inputs: []PromotionInput{promotionInputFixture(t, "mem_pa", "sha256_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "root_a"), promotionInputFixture(t, "mem_pb", "sha256_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "root_b")}}
	plan, err := BuildPromotionPlan(req)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.PromotionEligible || plan.SourceProjectCount != 2 || plan.Scope != ScopeGlobal || plan.ProposedGlobalMemoryID == "" {
		t.Fatalf("unexpected promotion plan: %+v", plan)
	}
	req.Inputs[1].TrustStatus = TrustGateBlocked
	if _, err := BuildPromotionPlan(req); err == nil {
		t.Fatal("blocked trust source must fail closed")
	}
	req.Inputs[1].TrustStatus = TrustGateTrusted
	req.Inputs[1].ProjectFamilyFingerprint = req.Inputs[0].ProjectFamilyFingerprint
	if _, err := BuildPromotionPlan(req); err == nil {
		t.Fatal("same project family must not count twice")
	}
	if _, err := BuildPromotionPlan(PromotionRequest{Policy: policy, PolicyRef: req.PolicyRef, Inputs: req.Inputs[:1]}); err == nil {
		t.Fatal("single project must not promote")
	}
}

func TestBuildPromotionPlanRejectsPolicyAndSourceDrift(t *testing.T) {
	policy := promotionPolicyFixture(t)
	h, _ := policy.ContentHash()
	req := PromotionRequest{Policy: policy, PolicyRef: PolicyRef{PolicyID: policy.PolicyID, PolicyType: policy.PolicyType, ContentSHA256: h}, Inputs: []PromotionInput{promotionInputFixture(t, "mem_pc", "sha256_cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "root_c"), promotionInputFixture(t, "mem_pd", "sha256_dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", "root_d")}}
	req.PolicyRef.ContentSHA256 = "sha256_ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if _, err := BuildPromotionPlan(req); err == nil {
		t.Fatal("policy hash drift must fail closed")
	}
	req.PolicyRef.ContentSHA256 = h
	req.Inputs[1].RootTaskIDs = req.Inputs[0].RootTaskIDs
	if _, err := BuildPromotionPlan(req); err == nil {
		t.Fatal("duplicate root task must fail closed")
	}
	req.Inputs[1].RootTaskIDs = []string{"root_d"}
	req.Inputs[1].EvidenceRefs = nil
	if _, err := BuildPromotionPlan(req); err == nil {
		t.Fatal("missing evidence must fail closed")
	}
}

func TestPromotionPlanIsDeterministic(t *testing.T) {
	policy := promotionPolicyFixture(t)
	h, _ := policy.ContentHash()
	a := promotionInputFixture(t, "mem_pe", "sha256_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "root_e")
	b := promotionInputFixture(t, "mem_pf", "sha256_ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", "root_f")
	p1, err := BuildPromotionPlan(PromotionRequest{Policy: policy, PolicyRef: PolicyRef{PolicyID: policy.PolicyID, PolicyType: policy.PolicyType, ContentSHA256: h}, Inputs: []PromotionInput{a, b}})
	if err != nil {
		t.Fatal(err)
	}
	p2, err := BuildPromotionPlan(PromotionRequest{Policy: policy, PolicyRef: PolicyRef{PolicyID: policy.PolicyID, PolicyType: policy.PolicyType, ContentSHA256: h}, Inputs: []PromotionInput{b, a}})
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := p1.CanonicalBytes()
	b2, _ := p2.CanonicalBytes()
	if string(b1) != string(b2) || p1.PlanHash() != p2.PlanHash() {
		t.Fatal("promotion plans must be byte stable")
	}
}
