package memory

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// MEM-02C 8.2/8.3/8.4: Trust Gate test matrix. Written before
// EvaluateEvidenceTrust existed (compile failure), then completed per plan.

const trustNow = "2026-08-08T00:00:00Z"

func trustNowTime() time.Time {
	t, err := time.Parse(time.RFC3339, trustNow)
	if err != nil {
		panic(err)
	}
	return t
}

func trustPolicy(allowed []string) PolicyFact {
	p := policyOf(PolicyTypeTrust)
	p.PolicyID = "trust_policy_v1"
	p.Config.Trust = &PolicyConfigTrust{
		AllowedAcquisitionMethods:            allowed,
		RequireProvenance:                    true,
		RequireVerificationStatus:            true,
		ExternalUnverifiedInstructionAllowed: false,
		PromotionRequiresPolicyEvidence:      true,
		Version:                              PolicyConfigSchemaVersion,
	}
	return fillPolicyHash(p)
}

func classifierPolicy() PolicyFact {
	p := policyOf(PolicyTypeContentClassifier)
	p.PolicyID = "content_classifier_v1"
	return fillPolicyHash(p)
}

// putClassification writes a content_classification judgment whose subject
// and payload match gen's enriched booleans and evidence ref.
func putClassification(t *testing.T, s *FactStore, gen MemoryEvidenceGeneration, classifier PolicyFact, id string) JudgmentFact {
	t.Helper()
	ref := gen.EvidenceRefs[0]
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    id,
		JudgmentType:  JudgmentTypeContentClassification,
		Scope:         ScopeProject,
		Subject:       JudgmentSubject{SubjectType: "evidence", EvidenceRef: &ref},
		Source:        JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		ContentClassification: &ContentClassificationPayload{
			EvidenceRef:                  ref,
			ContainsInstructionalContent: *gen.ContainsInstructionalContent,
			ContainsSensitiveContent:     *gen.ContainsSensitiveContent,
			ClassifierPolicyRef: PolicyRef{
				PolicyID: classifier.PolicyID, PolicyType: PolicyTypeContentClassifier, ContentSHA256: classifier.ContentSHA256,
			},
		},
		BasisRefs: []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
	}
	j = fillJudgmentHash(j)
	if _, err := s.Put(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	return j
}

// trustWorld builds a store with a revision and both policies. The evidence
// generation and classification judgment are per-test so mutated variants
// never collide with a pre-put identity.
func trustWorld(t *testing.T, key string) (*FactStore, PolicyFact, PolicyFact, MemoryRevision) {
	t.Helper()
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	if _, err := s.Put(context.Background(), rev); err != nil {
		t.Fatal(err)
	}
	trustPol := trustPolicy([]string{"direct", "tool_observed", "model_extracted", "imported"})
	if _, err := s.Put(context.Background(), trustPol); err != nil {
		t.Fatal(err)
	}
	classPol := classifierPolicy()
	if _, err := s.Put(context.Background(), classPol); err != nil {
		t.Fatal(err)
	}
	return s, trustPol, classPol, rev
}

func gateReq(gen MemoryEvidenceGeneration, trustPol PolicyFact, jud JudgmentFact, now time.Time) TrustGateRequest {
	return TrustGateRequest{
		Scope:              ScopeProject,
		MemoryID:           gen.MemoryID,
		Revision:           gen.Revision,
		EvidenceGeneration: gen.EvidenceGeneration,
		EvidenceRef:        gen.EvidenceRefs[0],
		TrustPolicyRef: PolicyRef{
			PolicyID: trustPol.PolicyID, PolicyType: PolicyTypeTrust, ContentSHA256: trustPol.ContentSHA256,
		},
		ContentClassificationRef: JudgmentRef{
			Scope: jud.Scope, JudgmentType: JudgmentTypeContentClassification,
			JudgmentID: jud.JudgmentID, ContentSHA256: jud.ContentSHA256,
		},
		Now: now,
	}
}

func evalGate(t *testing.T, s *FactStore, req TrustGateRequest) (*TrustGateResult, error) {
	t.Helper()
	return EvaluateEvidenceTrust(context.Background(), s, req)
}

func TestTrustGateLegacyUnavailable(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	legacy := validEvidenceGeneration() // Legacy: six fields absent.
	if _, err := s.Put(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	trustPol := trustPolicy([]string{"direct"})
	if _, err := s.Put(context.Background(), trustPol); err != nil {
		t.Fatal(err)
	}
	req := TrustGateRequest{
		Scope: ScopeProject, MemoryID: legacy.MemoryID, Revision: legacy.Revision,
		EvidenceGeneration: legacy.EvidenceGeneration, EvidenceRef: legacy.EvidenceRefs[0],
		TrustPolicyRef: PolicyRef{PolicyID: trustPol.PolicyID, PolicyType: PolicyTypeTrust, ContentSHA256: trustPol.ContentSHA256},
		ContentClassificationRef: JudgmentRef{
			Scope: ScopeProject, JudgmentType: JudgmentTypeContentClassification,
			JudgmentID: "judgment_unused", ContentSHA256: testHash,
		},
		Now: trustNowTime(),
	}
	res, err := evalGate(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TrustGateUnavailable || res.InstructionalContentAllowed || res.PromotionEligible {
		t.Errorf("legacy must be unavailable, got %+v", res)
	}
}

func TestTrustGateStatusMatrix(t *testing.T) {
	strictRef := func() PolicyRef {
		strict := trustPolicy([]string{"direct", "tool_observed", "model_extracted"})
		strict.PolicyID = "trust_policy_strict"
		strict = fillPolicyHash(strict)
		return PolicyRef{PolicyID: strict.PolicyID, PolicyType: PolicyTypeTrust, ContentSHA256: strict.ContentSHA256}
	}()
	cases := []struct {
		name        string
		mutGen      func(*MemoryEvidenceGeneration)
		mutReq      func(*TrustGateRequest)
		want        TrustGateStatus
		wantInstr   bool
		wantPromote bool
	}{
		{"verified trusted", func(g *MemoryEvidenceGeneration) {
			g.VerificationStatus = "verified"
		}, nil, TrustGateTrusted, true, true},
		{"confirmed trusted non instructional", func(g *MemoryEvidenceGeneration) {
			g.VerificationStatus = "confirmed"
			f := false
			g.ContainsInstructionalContent = &f
		}, nil, TrustGateTrusted, false, true},
		{"inferred restricted", func(g *MemoryEvidenceGeneration) {
			g.VerificationStatus = "inferred"
		}, nil, TrustGateRestricted, false, false},
		{"unverified unverified", func(g *MemoryEvidenceGeneration) {
			g.VerificationStatus = "unverified"
			g.EvidenceOrigin = "user"
		}, nil, TrustGateUnverified, false, false},
		{"method not allowed blocked", func(g *MemoryEvidenceGeneration) {
			g.AcquisitionMethod = "imported"
		}, func(r *TrustGateRequest) {
			r.TrustPolicyRef = strictRef
		}, TrustGateBlocked, false, false},
		{"sensitive blocked", func(g *MemoryEvidenceGeneration) {
			tr, fa := true, false
			g.ContainsSensitiveContent = &tr
			g.ContainsInstructionalContent = &fa
		}, nil, TrustGateBlocked, false, false},
		{"external unverified instructional blocked", func(g *MemoryEvidenceGeneration) {
			tr := true
			g.EvidenceOrigin = "external"
			g.VerificationStatus = "unverified"
			g.ContainsInstructionalContent = &tr
		}, nil, TrustGateBlocked, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, trustPol, classPol, _ := trustWorld(t, "matrix")
			gen := enrichedEvidence(validEvidenceGeneration())
			if c.mutGen != nil {
				c.mutGen(&gen)
			}
			gen = fillEvidenceHash(gen)
			if _, err := s.Put(context.Background(), gen); err != nil {
				t.Fatal(err)
			}
			jud := putClassification(t, s, gen, classPol, "judgment_class_matrix")
			if c.name == "method not allowed blocked" {
				strict := trustPolicy([]string{"direct", "tool_observed", "model_extracted"})
				strict.PolicyID = "trust_policy_strict"
				strict = fillPolicyHash(strict)
				if _, err := s.Put(context.Background(), strict); err != nil {
					t.Fatal(err)
				}
			}
			req := gateReq(gen, trustPol, jud, trustNowTime())
			if c.mutReq != nil {
				c.mutReq(&req)
			}
			res, err := evalGate(t, s, req)
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != c.want {
				t.Errorf("status = %s, want %s", res.Status, c.want)
			}
			if res.InstructionalContentAllowed != c.wantInstr {
				t.Errorf("instructional_content_allowed = %v, want %v", res.InstructionalContentAllowed, c.wantInstr)
			}
			if res.PromotionEligible != c.wantPromote {
				t.Errorf("promotion_eligible = %v, want %v", res.PromotionEligible, c.wantPromote)
			}
		})
	}
}

func TestTrustGateDeterministicOutput(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "det")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_det")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	res1, err := evalGate(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := evalGate(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	encoded1, err := res1.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	encoded2, err := res2.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded1, encoded2) {
		t.Errorf("same input must give byte-identical results: %s vs %s", encoded1, encoded2)
	}
	// Different Now only changes evaluated_at.
	req2 := req
	req2.Now = trustNowTime().Add(24 * time.Hour)
	res3, err := evalGate(t, s, req2)
	if err != nil {
		t.Fatal(err)
	}
	if res3.Status != res1.Status || res3.EvaluatedAt == res1.EvaluatedAt {
		t.Errorf("later Now must keep status and change evaluated_at only")
	}
	normalized := *res3
	normalized.EvaluatedAt = res1.EvaluatedAt
	encoded3, err := normalized.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encoded1, encoded3) {
		t.Errorf("different Now changed fields other than evaluated_at: %s vs %s", encoded1, encoded3)
	}
}

// ---- 8.3 references, corruption and time ----

func TestTrustGateEvidenceRefNotInGenerationFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "reffail")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_reffail")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	req.EvidenceRef = EvidenceRef{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_999", ContentSHA256: testHash}
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("evidence ref outside the generation must fail closed")
	}
}

func TestTrustGateGenerationIdentityMismatchFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "genmiss")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_genmiss")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	req.EvidenceGeneration = 99 // no such generation
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("missing generation must fail closed")
	}
}

func TestTrustGateGenerationBodyIdentityMismatchFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*MemoryEvidenceGeneration)
	}{
		{"memory_id", func(g *MemoryEvidenceGeneration) { g.MemoryID = "mem_other" }},
		{"revision", func(g *MemoryEvidenceGeneration) { g.Revision++ }},
		{"evidence_generation", func(g *MemoryEvidenceGeneration) { g.EvidenceGeneration++ }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, trustPol, classPol, _ := trustWorld(t, "body_identity")
			requested := enrichedEvidence(validEvidenceGeneration())
			stored := requested
			tc.mut(&stored)
			stored = fillEvidenceHash(stored)
			raw, err := stored.EncodeCanonical()
			if err != nil {
				t.Fatal(err)
			}
			key := memoryEvidenceKey(requested.MemoryID, requested.Revision, requested.EvidenceGeneration)
			comps, err := validateFactKey(key)
			if err != nil {
				t.Fatal(err)
			}
			path, err := secureJoin(s.root, factPathComps(FactKindMemoryEvidenceGeneration, comps), true, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			jud := putClassification(t, s, stored, classPol, "judgment_class_body_identity")
			req := gateReq(requested, trustPol, jud, trustNowTime())
			if _, err := evalGate(t, s, req); err == nil {
				t.Fatal("generation path and body identity mismatch must fail closed")
			}
		})
	}
}

func TestTrustGatePolicyMismatchFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "polmiss")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_polmiss")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	// Wrong hash: drifted policy must fail closed.
	req.TrustPolicyRef.ContentSHA256 = "sha256_" + strings.Repeat("a", 64)
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("policy hash drift must fail closed")
	}
	req2 := gateReq(gen, trustPol, jud, trustNowTime())
	req2.TrustPolicyRef.PolicyType = PolicyTypeFreshness
	if _, err := evalGate(t, s, req2); err == nil {
		t.Fatal("policy type mismatch must fail closed")
	}
}

func TestTrustGateLegacyFreeIdentifierPolicyFailClosed(t *testing.T) {
	// MEM-01C-era policy with free acquisition identifiers: readable, but
	// the Gate must refuse it as a security policy without changing its hash.
	s, _, classPol, _ := trustWorld(t, "freepol")
	legacyPol := policyOf(PolicyTypeTrust) // default: free identifiers
	legacyPol.PolicyID = "trust_policy_legacy_free"
	legacyPol = fillPolicyHash(legacyPol)
	if _, err := s.Put(context.Background(), legacyPol); err != nil {
		t.Fatal(err)
	}
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_freepol")
	req := gateReq(gen, legacyPol, jud, trustNowTime())
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("legacy free-identifier trust policy must fail closed")
	}
}

func TestTrustGateClassificationRefMismatchFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "classmiss")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_classmiss")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	req.ContentClassificationRef.ContentSHA256 = "sha256_" + strings.Repeat("b", 64)
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("classification ref hash mismatch must fail closed")
	}
	req2 := gateReq(gen, trustPol, jud, trustNowTime())
	req2.ContentClassificationRef.JudgmentType = JudgmentTypeConfirmation
	if _, err := evalGate(t, s, req2); err == nil {
		t.Fatal("classification ref type mismatch must fail closed")
	}
	req3 := gateReq(gen, trustPol, jud, trustNowTime())
	req3.ContentClassificationRef.Scope = ScopeGlobal
	if _, err := evalGate(t, s, req3); err == nil {
		t.Fatal("classification ref scope mismatch must fail closed")
	}
}

func TestTrustGateClassificationSubjectPayloadMismatchFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "subjmiss")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	// Judgment whose subject evidence differs from its payload evidence.
	ref := gen.EvidenceRefs[0]
	other := ref
	other.EvidenceID = "episode_other"
	j := JudgmentFact{
		SchemaVersion: 1, JudgmentID: "judgment_class_subjmiss", JudgmentType: JudgmentTypeContentClassification,
		Scope:   ScopeProject,
		Subject: JudgmentSubject{SubjectType: "evidence", EvidenceRef: &other},
		Source:  JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		ContentClassification: &ContentClassificationPayload{
			EvidenceRef:                  ref,
			ContainsInstructionalContent: *gen.ContainsInstructionalContent,
			ContainsSensitiveContent:     *gen.ContainsSensitiveContent,
			ClassifierPolicyRef:          PolicyRef{PolicyID: classPol.PolicyID, PolicyType: PolicyTypeContentClassifier, ContentSHA256: classPol.ContentSHA256},
		},
		BasisRefs: []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
	}
	j = fillJudgmentHash(j)
	if _, err := s.Put(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	req := gateReq(gen, trustPol, j, trustNowTime())
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("subject/payload evidence mismatch must fail closed")
	}
}

func TestTrustGateBooleanMismatchWithGenerationFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "boolmiss")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	// Judgment flips the sensitive boolean vs the generation.
	ref := gen.EvidenceRefs[0]
	flipped := !*gen.ContainsSensitiveContent
	j := JudgmentFact{
		SchemaVersion: 1, JudgmentID: "judgment_class_boolmiss", JudgmentType: JudgmentTypeContentClassification,
		Scope:   ScopeProject,
		Subject: JudgmentSubject{SubjectType: "evidence", EvidenceRef: &ref},
		Source:  JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		ContentClassification: &ContentClassificationPayload{
			EvidenceRef:                  ref,
			ContainsInstructionalContent: *gen.ContainsInstructionalContent,
			ContainsSensitiveContent:     flipped,
			ClassifierPolicyRef:          PolicyRef{PolicyID: classPol.PolicyID, PolicyType: PolicyTypeContentClassifier, ContentSHA256: classPol.ContentSHA256},
		},
		BasisRefs: []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
	}
	j = fillJudgmentHash(j)
	if _, err := s.Put(context.Background(), j); err != nil {
		t.Fatal(err)
	}
	req := gateReq(gen, trustPol, j, trustNowTime())
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("generation/judgment boolean mismatch must fail closed")
	}
}

func TestTrustGateClassifierPolicyMissingFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "classpolmiss")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_classpolmiss")
	// The classifier policy in the judgment points at a non-existent hash.
	jud.ContentClassification.ClassifierPolicyRef.ContentSHA256 = "sha256_" + strings.Repeat("c", 64)
	jud = fillJudgmentHash(jud)
	// Rewrite the judgment on disk with the drifted classifier ref.
	raw, err := jud.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	comps, err := validateFactKey(jud.JudgmentID)
	if err != nil {
		t.Fatal(err)
	}
	path, err := secureJoin(s.root, factPathComps(FactKindJudgment, comps), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	req := gateReq(gen, trustPol, jud, trustNowTime())
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("missing classifier policy must fail closed")
	}
}

func TestTrustGateZeroNowFailClosed(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "zeronow")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_zeronow")
	req := gateReq(gen, trustPol, jud, time.Time{})
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("zero now must fail closed")
	}
}

func TestTrustGateLegacyValidatesGenerationTimeBeforeUnavailable(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	legacy := validEvidenceGeneration()
	legacy.CreatedAt = "2026-08-09T00:00:00Z"
	legacy = fillEvidenceHash(legacy)
	if _, err := s.Put(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	req := TrustGateRequest{
		Scope: ScopeProject, MemoryID: legacy.MemoryID, Revision: legacy.Revision,
		EvidenceGeneration: legacy.EvidenceGeneration, EvidenceRef: legacy.EvidenceRefs[0],
		TrustPolicyRef: PolicyRef{PolicyID: "missing_trust", PolicyType: PolicyTypeTrust, ContentSHA256: testHash},
		ContentClassificationRef: JudgmentRef{
			Scope: ScopeProject, JudgmentType: JudgmentTypeContentClassification,
			JudgmentID: "missing_classification", ContentSHA256: testHash,
		},
		Now: trustNowTime(),
	}
	if _, err := evalGate(t, s, req); ErrorCode(err) != CodeEvaluationFutureReference {
		t.Fatalf("future legacy generation must fail with future_reference, got %v", err)
	}
}

func TestTrustGateLegacyDoesNotRequireClassificationOrPolicyFacts(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	legacy := validEvidenceGeneration()
	if _, err := s.Put(context.Background(), legacy); err != nil {
		t.Fatal(err)
	}
	req := TrustGateRequest{
		Scope: ScopeProject, MemoryID: legacy.MemoryID, Revision: legacy.Revision,
		EvidenceGeneration: legacy.EvidenceGeneration, EvidenceRef: legacy.EvidenceRefs[0],
		TrustPolicyRef: PolicyRef{PolicyID: "missing_trust", PolicyType: PolicyTypeTrust, ContentSHA256: testHash},
		ContentClassificationRef: JudgmentRef{
			Scope: ScopeProject, JudgmentType: JudgmentTypeContentClassification,
			JudgmentID: "missing_classification", ContentSHA256: testHash,
		},
		Now: trustNowTime(),
	}
	res, err := evalGate(t, s, req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != TrustGateUnavailable || res.InstructionalContentAllowed || res.PromotionEligible {
		t.Fatalf("legacy result = %+v, want unavailable/false/false", res)
	}
}

func TestTrustGateFutureReferencesFailClosed(t *testing.T) {
	// All facts are created_at 2026-08-07; a Now before that is a future ref.
	s, trustPol, classPol, _ := trustWorld(t, "future")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_future")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	req.Now = trustNowTime().AddDate(0, 0, -10)
	if _, err := evalGate(t, s, req); err == nil {
		t.Fatal("future references must fail closed")
	}
}

func TestTrustGateRedactedErrors(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "redact")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_redact")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	req.EvidenceRef.EvidenceID = "episode_with_secret_token"
	_, err := evalGate(t, s, req)
	if err == nil {
		t.Fatal("invalid evidence ref must fail closed")
	}
	if !IsSensitiveError(err) {
		t.Fatalf("error must be a redacted StoreError, got %T %v", err, err)
	}
	for _, leak := range []string{"/Users", "/var/", "secret", "tx_", req.EvidenceRef.EvidenceID} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("error leaks %q: %v", leak, err)
		}
	}
}

// ---- 8.4 isolation regressions ----

func TestTrustGateZeroWrites(t *testing.T) {
	s, trustPol, classPol, _ := trustWorld(t, "nowrite")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_nowrite")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	// Baseline before any evaluation: the gate must not add a single file.
	baseline := fileCount(t, s.root)
	if _, err := evalGate(t, s, req); err != nil {
		t.Fatal(err)
	}
	if got := fileCount(t, s.root); got != baseline {
		t.Errorf("gate evaluation must not write files: baseline=%d after first eval=%d", baseline, got)
	}
	if _, err := evalGate(t, s, req); err != nil {
		t.Fatal(err)
	}
	if got := fileCount(t, s.root); got != baseline {
		t.Errorf("repeated evaluation must not write files: baseline=%d after second eval=%d", baseline, got)
	}
}

func fileCount(t *testing.T, root string) int {
	t.Helper()
	var count int
	if err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			count++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestTrustGateDoesNotChangeDerivedState(t *testing.T) {
	s, trustPol, classPol, rev := trustWorld(t, "isolation")
	gen := enrichedEvidence(validEvidenceGeneration())
	if _, err := s.Put(context.Background(), gen); err != nil {
		t.Fatal(err)
	}
	jud := putClassification(t, s, gen, classPol, "judgment_class_isolation")
	req := gateReq(gen, trustPol, jud, trustNowTime())
	before, err := DeriveState(context.Background(), s, DerivedStateRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evalGate(t, s, req); err != nil {
		t.Fatal(err)
	}
	after, err := DeriveState(context.Background(), s, DerivedStateRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if string(statesFingerprint(before)) != string(statesFingerprint(after)) {
		t.Error("trust gate must not change derived state")
	}
	// evidence_validated keeps probation: the revision lifecycle is not
	// promoted by trusted evidence.
	_ = rev
}

func statesFingerprint(res *DerivedStateResult) []byte {
	var out []byte
	for _, st := range res.States {
		out = append(out, st.SnapshotBytes()...)
		out = append(out, '\n')
	}
	return out
}

func TestTrustGateResultCanonicalBytesStable(t *testing.T) {
	for _, status := range []TrustGateStatus{
		TrustGateTrusted,
		TrustGateRestricted,
		TrustGateUnverified,
		TrustGateBlocked,
		TrustGateUnavailable,
	} {
		result := TrustGateResult{
			Status:                      status,
			InstructionalContentAllowed: status == TrustGateTrusted,
			PromotionEligible:           status == TrustGateTrusted,
			EvaluatedAt:                 trustNow,
		}
		first, err := result.EncodeCanonical()
		if err != nil {
			t.Fatal(err)
		}
		second, err := result.EncodeCanonical()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("status %s encoding is not byte-stable", status)
		}
	}
}
