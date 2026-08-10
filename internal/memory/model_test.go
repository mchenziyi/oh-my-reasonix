package memory

import (
	"math"
	"strings"
	"testing"
)

func nanValue() float64 { return math.NaN() }
func infValue() float64 { return math.Inf(1) }

func TestUsagePolicyAllowedMatrix(t *testing.T) {
	allowed := map[MemoryType][]UsagePolicy{
		MemoryTypeStrategy:       {UsagePolicyOutcomeAttributed},
		MemoryTypePlaybook:       {UsagePolicyOutcomeAttributed},
		MemoryTypeComponent:      {UsagePolicyEvidenceValidated},
		MemoryTypePattern:        {UsagePolicyEvidenceValidated},
		MemoryTypeFailureConcept: {UsagePolicyEvidenceValidated},
		MemoryTypePreference:     {UsagePolicyExplicitConfirmation},
		MemoryTypeDecision:       {UsagePolicyEvidenceValidated, UsagePolicyExplicitConfirmation},
	}
	allPolicies := []UsagePolicy{UsagePolicyOutcomeAttributed, UsagePolicyEvidenceValidated, UsagePolicyExplicitConfirmation}
	for mt, policies := range allowed {
		for _, p := range policies {
			if !usagePolicyAllowed(mt, p) {
				t.Errorf("MemoryType %q should allow %q", mt, p)
			}
		}
		for _, p := range allPolicies {
			if !contains(policies, p) && usagePolicyAllowed(mt, p) {
				t.Errorf("MemoryType %q should reject %q", mt, p)
			}
		}
	}
}

func contains(list []UsagePolicy, p UsagePolicy) bool {
	for _, x := range list {
		if x == p {
			return true
		}
	}
	return false
}

func validRevision() MemoryRevision {
	r := MemoryRevision{
		SchemaVersion: 1,
		MemoryID:      "mem_01K7A9X2",
		MemoryType:    MemoryTypeStrategy,
		Scope:         ScopeProject,
		CanonicalKey:  "verify-before-upgrade-retry",
		Revision:      2,
		UsagePolicy:   UsagePolicyOutcomeAttributed,
		Title:         "Verify Before Upgrade Retry",
		Summary:       "Check the asset source before retrying an upgrade.",
		AppliesWhen: []ApplicabilityCondition{
			{
				ConditionID: "condition_sqlite_driver",
				Subject:     ApplicabilitySubjectEnvironment,
				Field:       "sqlite_driver",
				Operator:    ApplicabilityOperatorEquals,
				Value:       StrConditionValue("modernc.org/sqlite"),
			},
		},
		Aliases:   []string{"verify before upgrade retry"},
		CreatedAt: "2026-08-07T00:00:00Z",
	}
	h, err := r.ContentHash()
	if err != nil {
		panic(err)
	}
	r.ContentSHA256 = h
	return r
}

func fillRevisionHash(r MemoryRevision) MemoryRevision {
	h, err := r.ContentHash()
	if err != nil {
		panic(err)
	}
	r.ContentSHA256 = h
	return r
}

func TestMemoryRevisionValidation(t *testing.T) {
	r := validRevision()
	if err := r.Validate(); err != nil {
		t.Fatalf("valid revision rejected: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*MemoryRevision)
	}{
		{"schema_version zero", func(r *MemoryRevision) { r.SchemaVersion = 0 }},
		{"schema_version 2", func(r *MemoryRevision) { r.SchemaVersion = 2 }},
		{"empty memory_id", func(r *MemoryRevision) { r.MemoryID = "" }},
		{"path memory_id", func(r *MemoryRevision) { r.MemoryID = "data/mem_1" }},
		{"invalid memory_type", func(r *MemoryRevision) { r.MemoryType = MemoryType("script") }},
		{"invalid scope", func(r *MemoryRevision) { r.Scope = Scope("local") }},
		{"empty canonical_key", func(r *MemoryRevision) { r.CanonicalKey = "" }},
		{"canonical_key with path", func(r *MemoryRevision) { r.CanonicalKey = "wiki/strategies/x" }},
		{"canonical_key with space", func(r *MemoryRevision) { r.CanonicalKey = "verify before retry" }},
		{"revision zero", func(r *MemoryRevision) { r.Revision = 0 }},
		{"invalid usage_policy", func(r *MemoryRevision) { r.UsagePolicy = UsagePolicy("validated") }},
		{"type-policy mismatch", func(r *MemoryRevision) { r.UsagePolicy = UsagePolicyEvidenceValidated }},
		{"empty title", func(r *MemoryRevision) { r.Title = "" }},
		{"overlong title", func(r *MemoryRevision) { r.Title = strings.Repeat("x", 201) }},
		{"overlong summary", func(r *MemoryRevision) { r.Summary = strings.Repeat("x", 20001) }},
		{"control char in title", func(r *MemoryRevision) { r.Title = "bad\x00title" }},
		{"overlong alias", func(r *MemoryRevision) { r.Aliases = []string{strings.Repeat("y", 201)} }},
		{"newline alias", func(r *MemoryRevision) { r.Aliases = []string{"a\nb"} }},
		{"duplicate condition_id", func(r *MemoryRevision) {
			c := r.AppliesWhen[0]
			r.AppliesWhen = []ApplicabilityCondition{c, c}
		}},
		{"too many conditions", func(r *MemoryRevision) {
			var conds []ApplicabilityCondition
			for i := 0; i < 65; i++ {
				conds = append(conds, ApplicabilityCondition{
					ConditionID: "cond_" + strings.Repeat("x", 1) + string(rune('a'+i%26)) + string(rune('0'+i%10)),
					Subject:     ApplicabilitySubjectEnvironment,
					Field:       "f" + string(rune('0'+i%10)),
					Operator:    ApplicabilityOperatorExists,
					Value:       BoolConditionValue(true),
				})
			}
			r.AppliesWhen = conds
		}},
		{"failure_concept_ref wrong type", func(r *MemoryRevision) {
			ref := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash}
			r.FailureConceptRefs = []MemoryRef{ref}
		}},
		{"bad created_at", func(r *MemoryRevision) { r.CreatedAt = "2026-13-99" }},
		{"empty hash", func(r *MemoryRevision) { r.ContentSHA256 = "" }},
		{"hash mismatch", func(r *MemoryRevision) { r.ContentSHA256 = "sha256_" + strings.Repeat("f", 64) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rev := r
			c.mut(&rev)
			if err := rev.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestExplicitConfirmationRequiresSource(t *testing.T) {
	confRef := ConfirmationSourceRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_01K", ContentSHA256: testHash}

	ok := validRevision()
	ok.MemoryType = MemoryTypeDecision
	ok.UsagePolicy = UsagePolicyExplicitConfirmation
	ok.ConfirmationSourceRef = &confRef
	ok = fillRevisionHash(ok)
	if err := ok.Validate(); err != nil {
		t.Fatalf("decision with confirmation source should be valid: %v", err)
	}

	missing := validRevision()
	missing.MemoryType = MemoryTypePreference
	missing.UsagePolicy = UsagePolicyExplicitConfirmation
	missing = fillRevisionHash(missing)
	if err := missing.Validate(); err == nil {
		t.Error("explicit_confirmation without confirmation_source_ref should be rejected")
	}

	borrowed := validRevision()
	borrowed.ConfirmationSourceRef = &confRef
	borrowed = fillRevisionHash(borrowed)
	if err := borrowed.Validate(); err == nil {
		t.Error("non-explicit policy with confirmation_source_ref should be rejected")
	}

	badType := validRevision()
	badType.MemoryType = MemoryTypeDecision
	badType.UsagePolicy = UsagePolicyExplicitConfirmation
	badSource := ConfirmationSourceRef{Scope: ScopeProject, JudgmentType: JudgmentTypeAttributionOverride, JudgmentID: "judgment_01K", ContentSHA256: testHash}
	badType.ConfirmationSourceRef = &badSource
	badType = fillRevisionHash(badType)
	if err := badType.Validate(); err == nil {
		t.Error("confirmation source with non-confirmation judgment_type should be rejected")
	}
}

func TestMemoryEvidenceGenerationValidation(t *testing.T) {
	ev := MemoryEvidenceGeneration{
		SchemaVersion:              1,
		MemoryID:                   "mem_01K7A9X2",
		Revision:                   2,
		EvidenceGeneration:         3,
		EvidenceRefs:               []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}},
		PreviousEvidenceGeneration: intPtr(2),
		TransactionID:              "tx_01K",
		CreatedAt:                  "2026-08-07T00:00:00Z",
	}
	h, err := ev.ContentHash()
	if err != nil {
		panic(err)
	}
	ev.EvidenceSetSHA256 = h
	if err := ev.Validate(); err != nil {
		t.Fatalf("valid evidence generation rejected: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*MemoryEvidenceGeneration)
	}{
		{"schema_version zero", func(e *MemoryEvidenceGeneration) { e.SchemaVersion = 0 }},
		{"empty memory_id", func(e *MemoryEvidenceGeneration) { e.MemoryID = "" }},
		{"revision zero", func(e *MemoryEvidenceGeneration) { e.Revision = 0 }},
		{"evidence_generation zero", func(e *MemoryEvidenceGeneration) { e.EvidenceGeneration = 0 }},
		{"previous not less", func(e *MemoryEvidenceGeneration) { e.PreviousEvidenceGeneration = intPtr(3) }},
		{"previous zero", func(e *MemoryEvidenceGeneration) { e.PreviousEvidenceGeneration = intPtr(0) }},
		{"empty transaction_id", func(e *MemoryEvidenceGeneration) { e.TransactionID = "" }},
		{"path transaction_id", func(e *MemoryEvidenceGeneration) { e.TransactionID = "tx/../evil" }},
		{"invalid evidence_ref", func(e *MemoryEvidenceGeneration) {
			e.EvidenceRefs = []EvidenceRef{{Scope: Scope("x"), EvidenceType: "episode", EvidenceID: "e", ContentSHA256: testHash}}
		}},
		{"bad created_at", func(e *MemoryEvidenceGeneration) { e.CreatedAt = "not-a-time" }},
		{"empty hash", func(e *MemoryEvidenceGeneration) { e.EvidenceSetSHA256 = "" }},
		{"hash mismatch", func(e *MemoryEvidenceGeneration) { e.EvidenceSetSHA256 = "sha256_" + strings.Repeat("a", 64) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := ev
			c.mut(&e)
			if err := e.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func intPtr(i int) *int { return &i }

func validConfirmationJudgment() JudgmentFact {
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_01K",
		JudgmentType:  JudgmentTypeConfirmation,
		Scope:         ScopeProject,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef:   &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypePreference, MemoryID: "mem_pref_01K", Revision: 1, ContentSHA256: testHash},
		},
		Source: JudgmentSource{SourceType: "user", SourceID: "local_user"},
		Confirmation: &ConfirmationPayload{
			Status:        "confirmed",
			DeclaredScope: ScopeProject,
		},
		BasisRefs: []BasisRef{},
		CreatedAt: "2026-08-07T00:00:00Z",
	}
	h, err := j.ContentHash()
	if err != nil {
		panic(err)
	}
	j.ContentSHA256 = h
	return j
}

func TestJudgmentFactConfirmationValidation(t *testing.T) {
	j := validConfirmationJudgment()

	// a. confirmed + no supersedes is valid (baseline).
	if err := j.Validate(); err != nil {
		t.Fatalf("valid confirmation judgment rejected: %v", err)
	}

	// b. revoked + correct confirmation supersedes is valid. The revoked
	// fact must own a fresh ConfirmationPayload: mutating the shared
	// baseline pointer would pollute every later assertion.
	revoked := cloneConfirmation(j)
	revoked.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: j.Confirmation.DeclaredScope}
	revoked.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_02", ContentSHA256: testHash}
	revoked = fillJudgmentHash(revoked)
	if err := revoked.Validate(); err != nil {
		t.Errorf("revoked confirmation with supersedes should be valid: %v", err)
	}

	// c. revoked without supersedes is invalid.
	revokedNoSuper := cloneConfirmation(j)
	revokedNoSuper.Confirmation = &ConfirmationPayload{Status: "revoked", DeclaredScope: j.Confirmation.DeclaredScope}
	if err := revokedNoSuper.Validate(); err == nil {
		t.Error("revoked confirmation without supersedes should be rejected")
	}

	// d. supersedes of the wrong judgment type is invalid.
	wrongSuper := cloneConfirmation(j)
	wrongSuper.SupersedesJudgmentRef = &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeAttributionOverride, JudgmentID: "judgment_02", ContentSHA256: testHash}
	if err := wrongSuper.Validate(); err == nil {
		t.Error("supersedes of wrong judgment type should be rejected")
	}

	// The baseline must still be a confirmed fact after all the mutations
	// above; each table case starts from its own clone.
	if err := j.Validate(); err != nil {
		t.Fatalf("baseline polluted by earlier mutations: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*JudgmentFact)
	}{
		{"schema_version zero", func(j *JudgmentFact) { j.SchemaVersion = 0 }},
		{"empty judgment_id", func(j *JudgmentFact) { j.JudgmentID = "" }},
		{"invalid judgment_type", func(j *JudgmentFact) { j.JudgmentType = JudgmentType("critic") }},
		{"invalid scope", func(j *JudgmentFact) { j.Scope = Scope("x") }},
		{"bad subject", func(j *JudgmentFact) { j.Subject.SubjectType = "nonsense" }},
		{"bad source_type", func(j *JudgmentFact) { j.Source.SourceType = "user name" }},
		{"empty source_id", func(j *JudgmentFact) { j.Source.SourceID = "" }},
		{"bad confirmation status", func(j *JudgmentFact) { j.Confirmation.Status = "maybe" }},
		{"bad declared_scope", func(j *JudgmentFact) { j.Confirmation.DeclaredScope = Scope("global_wide") }},
		{"bad created_at", func(j *JudgmentFact) { j.CreatedAt = "yesterday" }},
		{"hash mismatch", func(j *JudgmentFact) { j.ContentSHA256 = "sha256_" + strings.Repeat("b", 64) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			jj := cloneConfirmation(j)
			c.mut(&jj)
			if err := jj.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

// cloneConfirmation returns an independent copy of a confirmation judgment:
// every pointer field is cloned so mutations cannot leak back into the
// shared baseline.
func cloneConfirmation(j JudgmentFact) JudgmentFact {
	c := j
	if j.Confirmation != nil {
		payload := *j.Confirmation
		c.Confirmation = &payload
	}
	if j.Subject.MemoryRef != nil {
		ref := *j.Subject.MemoryRef
		c.Subject.MemoryRef = &ref
	}
	if j.SupersedesJudgmentRef != nil {
		ref := *j.SupersedesJudgmentRef
		c.SupersedesJudgmentRef = &ref
	}
	return c
}

func fillJudgmentHash(j JudgmentFact) JudgmentFact {
	h, err := j.ContentHash()
	if err != nil {
		panic(err)
	}
	j.ContentSHA256 = h
	return j
}

func TestAttributionOverrideEffectEnum(t *testing.T) {
	validEffects := []string{"helped", "neutral", "harmed", "unknown"}
	for _, prev := range validEffects {
		for _, next := range validEffects {
			p := AttributionOverridePayload{PreviousEffect: prev, NewEffect: next, Reason: "review"}
			if err := p.Validate(); err != nil {
				t.Errorf("effect pair %q/%q should be valid: %v", prev, next, err)
			}
		}
	}
	invalid := []string{"", "bad", "HARMED", "helped!", "helpful", "unassessed", "likely", "affected"}
	for _, e := range invalid {
		p := AttributionOverridePayload{PreviousEffect: e, NewEffect: "neutral", Reason: "review"}
		if err := p.Validate(); err == nil {
			t.Errorf("previous_effect %q should be rejected", e)
		}
		p = AttributionOverridePayload{PreviousEffect: "neutral", NewEffect: e, Reason: "review"}
		if err := p.Validate(); err == nil {
			t.Errorf("new_effect %q should be rejected", e)
		}
	}
}

func TestAttributionOverrideEffectRejectedThroughJudgment(t *testing.T) {
	// Full judgment path: an unregistered effect must fail schema
	// validation before hash verification.
	j := validConfirmationJudgment()
	j.JudgmentType = JudgmentTypeAttributionOverride
	j.Confirmation = nil
	j.AttributionOverride = &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "no_effect", Reason: "x"}
	if err := j.Validate(); err == nil {
		t.Error("judgment with unregistered new_effect should be rejected")
	}
}

func TestJudgmentFactPayloadUnion(t *testing.T) {
	payloads := []struct {
		jt    JudgmentType
		build func() JudgmentFact
	}{
		{JudgmentTypeAttributionOverride, func() JudgmentFact {
			j := validConfirmationJudgment()
			j.JudgmentType = JudgmentTypeAttributionOverride
			j.Confirmation = nil
			j.AttributionOverride = &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "neutral", Reason: "third-party service failure"}
			return fillJudgmentHash(j)
		}},
		{JudgmentTypeRetrievalRelevance, func() JudgmentFact {
			j := validConfirmationJudgment()
			j.JudgmentType = JudgmentTypeRetrievalRelevance
			j.Confirmation = nil
			j.RetrievalRelevance = &RetrievalRelevancePayload{
				Result:              "missed_relevant",
				ExpectedMemoryRefs:  []MemoryRef{{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash}},
				RetrievedMemoryRefs: []MemoryRef{},
				EvidenceRefs:        []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}},
			}
			return fillJudgmentHash(j)
		}},
		{JudgmentTypeContextApplicability, func() JudgmentFact {
			j := validConfirmationJudgment()
			j.JudgmentType = JudgmentTypeContextApplicability
			j.Confirmation = nil
			j.Subject = JudgmentSubject{
				SubjectType:      "context",
				MemoryRef:        &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash},
				TargetContextRef: "context_target_01K",
			}
			j.ContextApplicability = &ContextApplicabilityPayload{
				Result:               "conditionally_applicable",
				RequiredConditionIDs: []string{"condition_sqlite_driver"},
				EvidenceRefs:         []EvidenceRef{},
			}
			return fillJudgmentHash(j)
		}},
		{JudgmentTypeContentClassification, func() JudgmentFact {
			j := validConfirmationJudgment()
			j.JudgmentType = JudgmentTypeContentClassification
			j.Confirmation = nil
			j.ContentClassification = &ContentClassificationPayload{
				EvidenceRef:                  EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash},
				ContainsInstructionalContent: true,
				ContainsSensitiveContent:     false,
				ClassifierPolicyRef:          PolicyRef{PolicyID: "content_classifier_policy_v1", PolicyType: PolicyTypeContentClassifier, ContentSHA256: testHash},
			}
			return fillJudgmentHash(j)
		}},
		{JudgmentTypeEvidenceTrust, func() JudgmentFact {
			j := validConfirmationJudgment()
			j.JudgmentType = JudgmentTypeEvidenceTrust
			j.Confirmation = nil
			j.EvidenceTrust = &EvidenceTrustPayload{
				EvidenceRef:                 EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash},
				ContentClassificationRef:    JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeContentClassification, JudgmentID: "judgment_class_01", ContentSHA256: testHash},
				TrustPolicyRef:              PolicyRef{PolicyID: "trust_policy_v1", PolicyType: PolicyTypeTrust, ContentSHA256: testHash},
				EvaluatedAt:                 "2026-08-07T00:00:00Z",
				InstructionalContentAllowed: false,
				PromotionEligible:           false,
			}
			return fillJudgmentHash(j)
		}},
		{JudgmentTypeFreshnessEvaluation, func() JudgmentFact {
			j := validConfirmationJudgment()
			j.JudgmentType = JudgmentTypeFreshnessEvaluation
			j.Confirmation = nil
			j.FreshnessEvaluation = &FreshnessEvaluationPayload{
				MemoryRef:          MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash},
				Result:             "aging",
				EvaluatedAt:        "2026-08-07T00:00:00Z",
				FreshnessPolicyRef: PolicyRef{PolicyID: "freshness_policy_v1", PolicyType: PolicyTypeFreshness, ContentSHA256: testHash},
				BasisRefs:          []BasisRef{},
			}
			return fillJudgmentHash(j)
		}},
	}
	for _, p := range payloads {
		t.Run(string(p.jt), func(t *testing.T) {
			j := p.build()
			if err := j.Validate(); err != nil {
				t.Fatalf("valid judgment rejected: %v", err)
			}
			// Mismatched union: keep payload but change judgment_type.
			// Payload validation runs before hash verification, so the
			// stale hash field is not what rejects this.
			j.JudgmentType = JudgmentTypeConfirmation
			if err := j.Validate(); err == nil {
				t.Error("payload union mismatch should be rejected")
			}
		})
	}
}

func TestJudgmentFactSubjectUnion(t *testing.T) {
	j := validConfirmationJudgment()
	// subject with no discriminator field: subject validation runs before
	// hash verification, so no re-hashing is needed for these rejections.
	empty := j
	empty.Subject = JudgmentSubject{}
	if err := empty.Validate(); err == nil {
		t.Error("subject without type should be rejected")
	}
	// outcome subject with missing outcome_id
	outcome := j
	outcome.Subject = JudgmentSubject{SubjectType: "memory_outcome"}
	if err := outcome.Validate(); err == nil {
		t.Error("memory_outcome subject without outcome_id should be rejected")
	}
	// context subject with missing target_context_ref
	ctx := j
	ctx.Subject = JudgmentSubject{SubjectType: "context", MemoryRef: &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash}}
	if err := ctx.Validate(); err == nil {
		t.Error("context subject without target_context_ref should be rejected")
	}
	// evidence subject with missing evidence_ref
	ev := j
	ev.Subject = JudgmentSubject{SubjectType: "evidence"}
	if err := ev.Validate(); err == nil {
		t.Error("evidence subject without evidence_ref should be rejected")
	}
	// valid outcome subject
	ok := j
	ok.Subject = JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: "outcome_01K"}
	ok = fillJudgmentHash(ok)
	if err := ok.Validate(); err != nil {
		t.Errorf("valid outcome subject rejected: %v", err)
	}
}

func TestContextApplicabilityConditionalRequiresConditions(t *testing.T) {
	j := validConfirmationJudgment()
	j.JudgmentType = JudgmentTypeContextApplicability
	j.Confirmation = nil
	j.Subject = JudgmentSubject{SubjectType: "context", MemoryRef: &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash}, TargetContextRef: "context_target_01K"}
	j.ContextApplicability = &ContextApplicabilityPayload{Result: "conditionally_applicable", RequiredConditionIDs: nil, EvidenceRefs: []EvidenceRef{}}
	// Payload validation runs before hash verification; the stale hash is
	// not what rejects this.
	if err := j.Validate(); err == nil {
		t.Error("conditionally_applicable without required_condition_ids should be rejected")
	}
}

func TestJudgmentSubtypeRefCrossChecks(t *testing.T) {
	// content_classification with wrong policy type. Payload validation
	// runs before hash verification, so no re-hashing is needed.
	j := validConfirmationJudgment()
	j.JudgmentType = JudgmentTypeContentClassification
	j.Confirmation = nil
	j.ContentClassification = &ContentClassificationPayload{
		EvidenceRef:                  EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash},
		ContainsInstructionalContent: false,
		ContainsSensitiveContent:     false,
		ClassifierPolicyRef:          PolicyRef{PolicyID: "trust_policy_v1", PolicyType: PolicyTypeTrust, ContentSHA256: testHash},
	}
	if err := j.Validate(); err == nil {
		t.Error("content_classification with non-classifier policy ref should be rejected")
	}

	// evidence_trust with wrong classification judgment type
	j2 := validConfirmationJudgment()
	j2.JudgmentType = JudgmentTypeEvidenceTrust
	j2.Confirmation = nil
	j2.EvidenceTrust = &EvidenceTrustPayload{
		EvidenceRef:                 EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash},
		ContentClassificationRef:    JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_01", ContentSHA256: testHash},
		TrustPolicyRef:              PolicyRef{PolicyID: "trust_policy_v1", PolicyType: PolicyTypeTrust, ContentSHA256: testHash},
		EvaluatedAt:                 "2026-08-07T00:00:00Z",
		InstructionalContentAllowed: false,
		PromotionEligible:           false,
	}
	if err := j2.Validate(); err == nil {
		t.Error("evidence_trust with non-classification ref should be rejected")
	}
}

func TestGovernanceEventValidation(t *testing.T) {
	ev := GovernanceEvent{
		SchemaVersion: 1,
		EventID:       "governance_01K",
		Scope:         ScopeProject,
		MemoryID:      "mem_01K7A9X2",
		Revision:      2,
		Operation:     "pin",
		Reason:        "user requested priority",
		Source:        "user",
		BasisRefs:     []BasisRef{},
		CreatedAt:     "2026-08-07T00:00:00Z",
	}
	if err := ev.Validate(); err != nil {
		t.Fatalf("valid governance event rejected: %v", err)
	}
	cases := []struct {
		name string
		mut  func(*GovernanceEvent)
	}{
		{"schema_version zero", func(e *GovernanceEvent) { e.SchemaVersion = 0 }},
		{"empty event_id", func(e *GovernanceEvent) { e.EventID = "" }},
		{"invalid scope", func(e *GovernanceEvent) { e.Scope = Scope("x") }},
		{"empty memory_id", func(e *GovernanceEvent) { e.MemoryID = "" }},
		{"revision zero", func(e *GovernanceEvent) { e.Revision = 0 }},
		{"invalid operation", func(e *GovernanceEvent) { e.Operation = "delete" }},
		{"path source", func(e *GovernanceEvent) { e.Source = "../user" }},
		{"bad created_at", func(e *GovernanceEvent) { e.CreatedAt = "now" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := ev
			c.mut(&e)
			if err := e.Validate(); err == nil {
				t.Error("expected validation error")
			}
		})
	}

	unfreeze := ev
	unfreeze.Operation = "unfreeze"
	if err := unfreeze.Validate(); err == nil {
		t.Error("unfreeze without basis_refs should be rejected")
	}
	// unfreeze basis_refs must be memory/evidence/judgment refs only.
	allowedRefs := []BasisRef{
		{MemoryRef: &MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypeStrategy, MemoryID: "mem_01", Revision: 1, ContentSHA256: testHash}},
		{EvidenceRef: &EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "ep_1", ContentSHA256: testHash}},
		{JudgmentRef: &JudgmentRef{Scope: ScopeProject, JudgmentType: JudgmentTypeAttributionOverride, JudgmentID: "judgment_01", ContentSHA256: testHash}},
	}
	for _, ref := range allowedRefs {
		u := unfreeze
		u.BasisRefs = []BasisRef{ref}
		if err := u.Validate(); err != nil {
			t.Errorf("unfreeze with %T basis should be valid: %v", ref, err)
		}
	}
	policyBasis := unfreeze
	policyBasis.BasisRefs = []BasisRef{{PolicyRef: &PolicyRef{PolicyID: "trust_policy_v1", PolicyType: PolicyTypeTrust, ContentSHA256: testHash}}}
	if err := policyBasis.Validate(); err == nil {
		t.Error("unfreeze with policy basis_ref should be rejected")
	}
	mixed := unfreeze
	mixed.BasisRefs = []BasisRef{allowedRefs[0], policyBasis.BasisRefs[0]}
	if err := mixed.Validate(); err == nil {
		t.Error("unfreeze with mixed policy basis_ref should be rejected")
	}
}

// TestDerivedStateNotFactField ensures lifecycle/health/usage fields are not
// accepted as fact fields (they are derived state).
func TestDerivedStateNotFactField(t *testing.T) {
	jsonInputs := []string{
		`{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z","lifecycle":"active"}`,
		`{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z","health":"healthy"}`,
		`{"schema_version":1,"memory_id":"mem_01","memory_type":"strategy","scope":"project","canonical_key":"k","revision":1,"usage_policy":"outcome_attributed","title":"T","summary":"S","applies_when":[],"does_not_apply_when":[],"failure_concept_refs":[],"relations":[],"aliases":[],"content_sha256":"` + testHash + `","created_at":"2026-08-07T00:00:00Z","usage_count":3}`,
	}
	for _, in := range jsonInputs {
		if _, err := DecodeStrict[MemoryRevision]([]byte(in)); err == nil {
			t.Error("derived-state field should be rejected as unknown")
		}
	}
}
