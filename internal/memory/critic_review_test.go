package memory

// MEM-02B failure-first tests: critic_review Judgment subtype (Schema layer).
// These tests reference JudgmentTypeCriticReview / CriticReviewPayload, which
// did not exist before MEM-02B, so the package failed to compile until the
// union branch landed.

import (
	"bytes"
	"encoding/json"
	"testing"
)

// ---- helpers ----

func validCriticEvidence() EvidenceRef {
	return EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}
}

func validCriticContext() MemoryContext {
	return MemoryContext{
		ProjectGenerationRef: &ProjectGenerationRef{
			SchemaVersion: 1, Scope: ScopeProject,
			GenerationID: "gen_project_000010", InputManifestID: "gen_project_000010",
			InputManifestSHA256: testHash,
		},
		GlobalGenerationRef: nil,
	}
}

// validCriticJudgment builds a passed critic_review judgment whose required
// evidence is fully contained in the envelope basis_refs.
func validCriticJudgment(rev MemoryRevision) JudgmentFact {
	ev := validCriticEvidence()
	base := ev
	return JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_critic_01",
		JudgmentType:  JudgmentTypeCriticReview,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		CriticReview: &CriticReviewPayload{
			Result:          "passed",
			EvaluationScope: "generation_full_scan",
			MemoryContext:   validCriticContext(),
			RequiredEvidenceRefs: []EvidenceRef{
				ev,
			},
		},
		BasisRefs: []BasisRef{
			{EvidenceRef: &base},
		},
		CreatedAt: "2026-08-11T00:00:00Z",
	}
}

func fillCriticHash(j JudgmentFact) JudgmentFact {
	h, err := j.ContentHash()
	if err != nil {
		panic(err)
	}
	j.ContentSHA256 = h
	return j
}

// ---- 2.2 Schema validation ----

func TestCriticReviewRoundTrip(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	for _, result := range []string{"passed", "failed", "unavailable"} {
		j := validCriticJudgment(rev)
		j.CriticReview.Result = result
		j = fillCriticHash(j)
		if err := j.Validate(); err != nil {
			t.Fatalf("critic_review %s rejected: %v", result, err)
		}
		raw, err := j.EncodeCanonical()
		if err != nil {
			t.Fatal(err)
		}
		back, err := DecodeStrict[JudgmentFact](raw)
		if err != nil {
			t.Fatalf("round-trip %s failed: %v", result, err)
		}
		if back.JudgmentType != JudgmentTypeCriticReview || back.CriticReview == nil {
			t.Fatal("critic_review branch lost in round trip")
		}
		if back.CriticReview.Result != result {
			t.Errorf("result = %q, want %q", back.CriticReview.Result, result)
		}
		if back.ContentSHA256 != j.ContentSHA256 {
			t.Error("content hash not stable across round trip")
		}
	}
}

func TestCriticReviewRejectsBadResultAndScope(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	cases := []struct {
		name string
		mut  func(*CriticReviewPayload)
	}{
		{"unknown result", func(p *CriticReviewPayload) { p.Result = "partially_passed" }},
		{"empty result", func(p *CriticReviewPayload) { p.Result = "" }},
		{"unknown evaluation_scope", func(p *CriticReviewPayload) { p.EvaluationScope = "universe_scan" }},
		{"empty evaluation_scope", func(p *CriticReviewPayload) { p.EvaluationScope = "" }},
		{"context both nil", func(p *CriticReviewPayload) { p.MemoryContext = MemoryContext{} }},
	}
	for _, tc := range cases {
		j := validCriticJudgment(rev)
		tc.mut(j.CriticReview)
		if err := j.Validate(); err == nil {
			t.Errorf("%s: must be rejected", tc.name)
		}
	}
}

func TestCriticReviewSourceEnum(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	valid := []string{"fixture_oracle", "offline_rule", "user_review"}
	for _, st := range valid {
		j := validCriticJudgment(rev)
		j.Source = JudgmentSource{SourceType: st, SourceID: "src_01"}
		j = fillCriticHash(j)
		if err := j.Validate(); err != nil {
			t.Errorf("critic source %q rejected: %v", st, err)
		}
	}
	for _, st := range []string{"user", "tool", "model", "agent_reported"} {
		j := validCriticJudgment(rev)
		j.Source = JudgmentSource{SourceType: st, SourceID: "src_01"}
		if err := j.Validate(); err == nil {
			t.Errorf("critic source %q must be rejected", st)
		}
	}
	// The critic-only source restriction must not tighten other subtypes.
	c := validConfirmationJudgment()
	c.Source = JudgmentSource{SourceType: "user", SourceID: "local_user"}
	if err := c.Validate(); err != nil {
		t.Errorf("non-critic judgment source must be unaffected: %v", err)
	}
}

func TestCriticReviewSubjectMustBeMemoryRevision(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	j := validCriticJudgment(rev)
	j.Subject = JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_01K"}
	if err := j.Validate(); err == nil {
		t.Error("critic_review with non-memory_revision subject must be rejected")
	}
	// subject scope must equal judgment scope.
	j = validCriticJudgment(rev)
	j.Subject.MemoryRef.Scope = ScopeGlobal
	if err := j.Validate(); err == nil {
		t.Error("critic_review with subject scope != judgment scope must be rejected")
	}
}

func TestCriticReviewPassedRequiresEvidenceAndBasis(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	// passed without required evidence.
	j := validCriticJudgment(rev)
	j.CriticReview.RequiredEvidenceRefs = nil
	if err := j.Validate(); err == nil {
		t.Error("passed without required evidence must be rejected")
	}
	// passed without basis.
	j = validCriticJudgment(rev)
	j.BasisRefs = nil
	if err := j.Validate(); err == nil {
		t.Error("passed without basis must be rejected")
	}
	// passed with required evidence missing from basis.
	j = validCriticJudgment(rev)
	other := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_2", ContentSHA256: testHash}
	j.CriticReview.RequiredEvidenceRefs = []EvidenceRef{other}
	ev := validCriticEvidence()
	j.BasisRefs = []BasisRef{{EvidenceRef: &ev}}
	if err := j.Validate(); err == nil {
		t.Error("passed with required evidence missing from basis must be rejected")
	}
	// failed / unavailable may omit required evidence but keep the rest valid.
	for _, result := range []string{"failed", "unavailable"} {
		j := validCriticJudgment(rev)
		j.CriticReview.Result = result
		j.CriticReview.RequiredEvidenceRefs = nil
		j.BasisRefs = nil
		j = fillCriticHash(j)
		if err := j.Validate(); err != nil {
			t.Errorf("%s without required evidence should be valid: %v", result, err)
		}
	}
}

func TestCriticReviewWrongUnionBranchRejected(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	// critic_review type with a different payload branch set.
	j := validCriticJudgment(rev)
	j.CriticReview = nil
	j.Confirmation = &ConfirmationPayload{Status: "confirmed", DeclaredScope: ScopeProject}
	if err := j.Validate(); err == nil {
		t.Error("critic_review with confirmation payload must be rejected")
	}
	// Exactly-one-payload rule with two branches.
	j = validCriticJudgment(rev)
	j.Confirmation = &ConfirmationPayload{Status: "confirmed", DeclaredScope: ScopeProject}
	if err := j.Validate(); err == nil {
		t.Error("two payload branches must be rejected")
	}
}

func TestCriticReviewStrictDecodeAndHash(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	j := validCriticJudgment(rev)
	j = fillCriticHash(j)
	raw, err := j.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	// Unknown field must be rejected.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["critic_extra"] = true
	injected, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeStrict[JudgmentFact](injected); err == nil {
		t.Fatal("unknown field must be rejected")
	}
	// Hash drift must be rejected.
	drift := validCriticJudgment(rev)
	drift.ContentSHA256 = "sha256_" + "bb"
	if err := drift.Validate(); err == nil {
		t.Fatal("hash drift must be rejected")
	}
}

func TestCriticReviewRequiredRefsDeterministic(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	ev1 := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}
	ev2 := EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_2", ContentSHA256: testHash}
	a := validCriticJudgment(rev)
	a.CriticReview.RequiredEvidenceRefs = []EvidenceRef{ev2, ev1, ev2}
	a.BasisRefs = []BasisRef{{EvidenceRef: &ev1}, {EvidenceRef: &ev2}}
	a = fillCriticHash(a)
	b := validCriticJudgment(rev)
	b.CriticReview.RequiredEvidenceRefs = []EvidenceRef{ev1, ev2}
	b.BasisRefs = []BasisRef{{EvidenceRef: &ev2}, {EvidenceRef: &ev1}}
	b = fillCriticHash(b)
	ba, err := a.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	bb, err := b.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(ba, bb) {
		t.Error("required evidence refs must be order-independent (sorted dedupe)")
	}
}

// ---- 2.3 old judgment golden compatibility ----

// The seven pre-MEM-02B subtypes must keep byte-identical canonical output
// and content hashes. The goldens were captured from the pre-MEM-02B build.
func TestPreExistingJudgmentGoldenCompatibility(t *testing.T) {
	rev := validRevision()
	cases := []struct {
		name  string
		build func() JudgmentFact
	}{
		{"confirmation", func() JudgmentFact {
			j := validConfirmationJudgment()
			return j
		}},
		{"attribution_override", func() JudgmentFact {
			j := JudgmentFact{
				SchemaVersion: 1, JudgmentID: "judgment_ao", JudgmentType: JudgmentTypeAttributionOverride,
				Scope:               ScopeProject,
				Subject:             JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_01K"},
				Source:              JudgmentSource{SourceType: "user", SourceID: "local_user"},
				AttributionOverride: &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "neutral", Reason: "third party"},
				BasisRefs:           []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
			}
			return fillJudgmentHash(j)
		}},
		{"retrieval_relevance", func() JudgmentFact {
			j := JudgmentFact{
				SchemaVersion: 1, JudgmentID: "judgment_rr", JudgmentType: JudgmentTypeRetrievalRelevance,
				Scope:              ScopeProject,
				Subject:            JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &MemoryRef{Scope: ScopeProject, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}},
				Source:             JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
				RetrievalRelevance: &RetrievalRelevancePayload{Result: "hit_relevant", ExpectedMemoryRefs: []MemoryRef{}, RetrievedMemoryRefs: []MemoryRef{}, EvidenceRefs: []EvidenceRef{}},
				BasisRefs:          []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
			}
			return fillJudgmentHash(j)
		}},
		{"context_applicability", func() JudgmentFact {
			j := JudgmentFact{
				SchemaVersion: 1, JudgmentID: "judgment_ca", JudgmentType: JudgmentTypeContextApplicability,
				Scope:                ScopeProject,
				Subject:              JudgmentSubject{SubjectType: "context", MemoryRef: &MemoryRef{Scope: ScopeProject, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}, TargetContextRef: "context_01K"},
				Source:               JudgmentSource{SourceType: "user", SourceID: "local_user"},
				ContextApplicability: &ContextApplicabilityPayload{Result: "applicable", RequiredConditionIDs: []string{}, EvidenceRefs: []EvidenceRef{}},
				BasisRefs:            []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
			}
			return fillJudgmentHash(j)
		}},
		{"content_classification", func() JudgmentFact {
			j := JudgmentFact{
				SchemaVersion: 1, JudgmentID: "judgment_cc", JudgmentType: JudgmentTypeContentClassification,
				Scope:                 ScopeProject,
				Subject:               JudgmentSubject{SubjectType: "evidence", EvidenceRef: &EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}},
				Source:                JudgmentSource{SourceType: "tool", SourceID: "classifier_01"},
				ContentClassification: &ContentClassificationPayload{EvidenceRef: EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}, ContainsInstructionalContent: true, ContainsSensitiveContent: false, ClassifierPolicyRef: PolicyRef{PolicyID: "content_classifier_policy_v1", PolicyType: PolicyTypeContentClassifier, ContentSHA256: testHash}},
				BasisRefs:             []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
			}
			return fillJudgmentHash(j)
		}},
		{"evidence_trust", func() JudgmentFact {
			j := JudgmentFact{
				SchemaVersion: 1, JudgmentID: "judgment_et", JudgmentType: JudgmentTypeEvidenceTrust,
				Scope:         ScopeProject,
				Subject:       JudgmentSubject{SubjectType: "evidence", EvidenceRef: &EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}},
				Source:        JudgmentSource{SourceType: "tool", SourceID: "trust_01"},
				EvidenceTrust: &EvidenceTrustPayload{EvidenceRef: EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}, ContentClassificationRef: JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeContentClassification, JudgmentID: "judgment_cc", ContentSHA256: testHash}, TrustPolicyRef: PolicyRef{PolicyID: "trust_policy_v1", PolicyType: PolicyTypeTrust, ContentSHA256: testHash}, EvaluatedAt: "2026-08-07T00:00:00Z", InstructionalContentAllowed: false, PromotionEligible: false},
				BasisRefs:     []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
			}
			return fillJudgmentHash(j)
		}},
		{"freshness_evaluation", func() JudgmentFact {
			j := JudgmentFact{
				SchemaVersion: 1, JudgmentID: "judgment_fe", JudgmentType: JudgmentTypeFreshnessEvaluation,
				Scope:               ScopeProject,
				Subject:             JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &MemoryRef{Scope: ScopeProject, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}},
				Source:              JudgmentSource{SourceType: "tool", SourceID: "freshness_01"},
				FreshnessEvaluation: &FreshnessEvaluationPayload{MemoryRef: MemoryRef{Scope: ScopeProject, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}, Result: "fresh", EvaluatedAt: "2026-08-07T00:00:00Z", FreshnessPolicyRef: PolicyRef{PolicyID: "freshness_policy_v1", PolicyType: PolicyTypeFreshness, ContentSHA256: testHash}, BasisRefs: []BasisRef{}},
				BasisRefs:           []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
			}
			return fillJudgmentHash(j)
		}},
	}
	goldenHash := map[string]string{
		"confirmation":           "sha256_747d1fe2053598622bea90f4b05114430b1070073ce19353071160dfd6e186da",
		"attribution_override":   "sha256_9ad33d8241f987d0a69bdc3341b29a593da9235bf8fb023355569c347c7944d5",
		"retrieval_relevance":    "sha256_0dc2aae0e9a1b8e9499acc3768a01187a1bb8e8cdd56927314a14ac33bfb2267",
		"context_applicability":  "sha256_ab8511b2af9aeb3a213ecec58c2f1fedaec7b46ea1d69caa6cff09eb6b925b50",
		"content_classification": "sha256_9bf2da9412f7e107a2c88195ff1df04869ac685346332067b53758c4031c0ab1",
		"evidence_trust":         "sha256_79dc05957fd82d0419ebd699d78197b90a8d5a8cf90dc2ac573a0e7b1117f3c9",
		"freshness_evaluation":   "sha256_f277ec4e47975d8582a3ddc205ac6429ba44d3dd57b20e8f298c11f2e7c61be8",
	}
	for _, tc := range cases {
		j := tc.build()
		if err := j.Validate(); err != nil {
			t.Fatalf("%s: valid judgment rejected: %v", tc.name, err)
		}
		h, err := j.ContentHash()
		if err != nil {
			t.Fatal(err)
		}
		if h != goldenHash[tc.name] {
			t.Errorf("%s: content hash changed: %s != %s", tc.name, h, goldenHash[tc.name])
		}
	}
}
