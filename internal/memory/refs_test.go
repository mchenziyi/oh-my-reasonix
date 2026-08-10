package memory

import (
	"strings"
	"testing"
)

var testHash = "sha256_" + strings.Repeat("0", 64)

func TestScopeValidate(t *testing.T) {
	valid := []Scope{ScopeProject, ScopeGlobal, ScopePortable}
	for _, s := range valid {
		if err := s.Validate(); err != nil {
			t.Errorf("Scope %q should be valid: %v", s, err)
		}
	}
	for _, s := range []Scope{"", "PROJECT", "local", "project/global"} {
		if err := s.Validate(); err == nil {
			t.Errorf("Scope %q should be rejected", s)
		}
	}
}

func TestMemoryTypeValidate(t *testing.T) {
	valid := []MemoryType{MemoryTypePattern, MemoryTypeStrategy, MemoryTypeDecision, MemoryTypePlaybook, MemoryTypePreference, MemoryTypeFailureConcept, MemoryTypeComponent}
	for _, m := range valid {
		if err := m.Validate(); err != nil {
			t.Errorf("MemoryType %q should be valid: %v", m, err)
		}
	}
	for _, m := range []MemoryType{"", "component_type", "script", "MEMORY"} {
		if err := m.Validate(); err == nil {
			t.Errorf("MemoryType %q should be rejected", m)
		}
	}
}

func TestUsagePolicyValidate(t *testing.T) {
	valid := []UsagePolicy{UsagePolicyOutcomeAttributed, UsagePolicyEvidenceValidated, UsagePolicyExplicitConfirmation}
	for _, p := range valid {
		if err := p.Validate(); err != nil {
			t.Errorf("UsagePolicy %q should be valid: %v", p, err)
		}
	}
	for _, p := range []UsagePolicy{"", "validated", "user_confirmed", "OUTCOME_ATTRIBUTED"} {
		if err := p.Validate(); err == nil {
			t.Errorf("UsagePolicy %q should be rejected", p)
		}
	}
}

func TestJudgmentTypeValidate(t *testing.T) {
	valid := []JudgmentType{JudgmentTypeConfirmation, JudgmentTypeAttributionOverride, JudgmentTypeRetrievalRelevance, JudgmentTypeContextApplicability, JudgmentTypeContentClassification, JudgmentTypeEvidenceTrust, JudgmentTypeFreshnessEvaluation}
	for _, j := range valid {
		if err := j.Validate(); err != nil {
			t.Errorf("JudgmentType %q should be valid: %v", j, err)
		}
	}
	for _, j := range []JudgmentType{"", "confirm", "generalization_critic", "CONFIRMATION"} {
		if err := j.Validate(); err == nil {
			t.Errorf("JudgmentType %q should be rejected", j)
		}
	}
}

func TestPolicyTypeValidate(t *testing.T) {
	valid := []PolicyType{PolicyTypeFreshness, PolicyTypeTrust, PolicyTypeContentClassifier, PolicyTypeIndex, PolicyTypeBenchmark}
	for _, p := range valid {
		if err := p.Validate(); err != nil {
			t.Errorf("PolicyType %q should be valid: %v", p, err)
		}
	}
	for _, p := range []PolicyType{"", "retention", "freshness_v2", "TRUST"} {
		if err := p.Validate(); err == nil {
			t.Errorf("PolicyType %q should be rejected", p)
		}
	}
}

func TestMemoryRefValidation(t *testing.T) {
	base := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01K7A9X2", Revision: 2, ContentSHA256: testHash}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid ref rejected: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*MemoryRef)
	}{
		{"empty scope", func(r *MemoryRef) { r.Scope = "" }},
		{"invalid scope", func(r *MemoryRef) { r.Scope = Scope("local") }},
		{"invalid memory_type", func(r *MemoryRef) { r.MemoryType = MemoryType("script") }},
		{"empty memory_id", func(r *MemoryRef) { r.MemoryID = "" }},
		{"path-like memory_id", func(r *MemoryRef) { r.MemoryID = "../mem" }},
		{"absolute path memory_id", func(r *MemoryRef) { r.MemoryID = "/data/mem_1" }},
		{"revision zero", func(r *MemoryRef) { r.Revision = 0 }},
		{"empty hash", func(r *MemoryRef) { r.ContentSHA256 = "" }},
		{"bare hash", func(r *MemoryRef) { r.ContentSHA256 = strings.Repeat("0", 64) }},
		{"bad hash prefix", func(r *MemoryRef) { r.ContentSHA256 = "md5_" + strings.Repeat("0", 32) }},
		{"bad hash hex", func(r *MemoryRef) { r.ContentSHA256 = "sha256_" + strings.Repeat("g", 64) }},
		{"short hash", func(r *MemoryRef) { r.ContentSHA256 = "sha256_abc" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := base
			c.mut(&r)
			if err := r.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestEvidenceRefValidation(t *testing.T) {
	base := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid ref rejected: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*EvidenceRef)
	}{
		{"empty scope", func(r *EvidenceRef) { r.Scope = "" }},
		{"empty evidence_type", func(r *EvidenceRef) { r.EvidenceType = "" }},
		{"path evidence_type", func(r *EvidenceRef) { r.EvidenceType = "/var/log" }},
		{"empty evidence_id", func(r *EvidenceRef) { r.EvidenceID = "" }},
		{"path evidence_id", func(r *EvidenceRef) { r.EvidenceID = "data/ev_1" }},
		{"invalid hash", func(r *EvidenceRef) { r.ContentSHA256 = "sha256_xyz" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := base
			c.mut(&r)
			if err := r.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestJudgmentRefValidation(t *testing.T) {
	base := JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeAttributionOverride, JudgmentID: "judgment_01K", ContentSHA256: testHash}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid ref rejected: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*JudgmentRef)
	}{
		{"empty scope", func(r *JudgmentRef) { r.Scope = "" }},
		{"invalid judgment_type", func(r *JudgmentRef) { r.JudgmentType = JudgmentType("critic") }},
		{"empty judgment_id", func(r *JudgmentRef) { r.JudgmentID = "" }},
		{"path judgment_id", func(r *JudgmentRef) { r.JudgmentID = "tmp/judgment_1" }},
		{"invalid hash", func(r *JudgmentRef) { r.ContentSHA256 = "sha256_zz" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := base
			c.mut(&r)
			if err := r.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestConfirmationSourceRefRequiresConfirmationType(t *testing.T) {
	base := ConfirmationSourceRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_01K", ContentSHA256: testHash}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid confirmation source rejected: %v", err)
	}
	for _, jt := range []JudgmentType{JudgmentTypeAttributionOverride, JudgmentTypeRetrievalRelevance, JudgmentTypeContextApplicability, JudgmentTypeContentClassification, JudgmentTypeEvidenceTrust, JudgmentTypeFreshnessEvaluation} {
		r := base
		r.JudgmentType = jt
		if err := r.Validate(); err == nil {
			t.Errorf("ConfirmationSourceRef with judgment_type %q should be rejected", jt)
		}
	}
}

func TestPolicyRefValidation(t *testing.T) {
	base := PolicyRef{PolicyID: "freshness_policy_v1", PolicyType: PolicyTypeFreshness, ContentSHA256: testHash}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid policy ref rejected: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*PolicyRef)
	}{
		{"empty policy_id", func(r *PolicyRef) { r.PolicyID = "" }},
		{"path policy_id", func(r *PolicyRef) { r.PolicyID = "cfg/policies/1" }},
		{"invalid policy_type", func(r *PolicyRef) { r.PolicyType = PolicyType("retention") }},
		{"invalid hash", func(r *PolicyRef) { r.ContentSHA256 = "sha256_" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := base
			c.mut(&r)
			if err := r.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestBasisRefExactlyOne(t *testing.T) {
	base := BasisRef{MemoryRef: &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash}}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid basis ref rejected: %v", err)
	}
	empty := BasisRef{}
	if err := empty.Validate(); err == nil {
		t.Error("basis ref with no ref should be rejected")
	}
	both := BasisRef{
		MemoryRef:   base.MemoryRef,
		EvidenceRef: &EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash},
	}
	if err := both.Validate(); err == nil {
		t.Error("basis ref with two refs should be rejected")
	}
	// Canonical paths must fail cleanly instead of panicking on a zero ref.
	if _, err := empty.canonMap(); err == nil {
		t.Error("zero basis ref canonMap should fail")
	}
	if _, err := both.canonMap(); err == nil {
		t.Error("multi-ref basis canonMap should fail")
	}
}
