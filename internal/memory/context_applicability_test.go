package memory

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func contextJudgment(t *testing.T, enriched bool, result string) JudgmentFact {
	t.Helper()
	rev := validRevision()
	j := JudgmentFact{
		SchemaVersion: SchemaVersion,
		JudgmentID:    "judgment_context_01",
		JudgmentType:  JudgmentTypeContextApplicability,
		Scope:         ScopeProject,
		Subject: JudgmentSubject{
			SubjectType: "context",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID,
				Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
			TargetContextRef: "context_target_01",
		},
		Source: JudgmentSource{SourceType: "user_review", SourceID: "review_01"},
		ContextApplicability: &ContextApplicabilityPayload{
			Result: result, RequiredConditionIDs: []string{}, EvidenceRefs: []EvidenceRef{},
		},
		BasisRefs: []BasisRef{}, CreatedAt: "2026-08-11T00:00:00Z",
	}
	if result == "conditionally_applicable" {
		j.ContextApplicability.RequiredConditionIDs = []string{"condition_sqlite_driver"}
	}
	if enriched {
		j.BasisContextRefs = []string{"context_basis_b", "context_basis_a"}
	}
	j.ContentSHA256, _ = j.ContentHash()
	return j
}

func TestContextApplicabilityLegacyGoldenUnchanged(t *testing.T) {
	rev := validRevision()
	j := JudgmentFact{
		SchemaVersion: SchemaVersion, JudgmentID: "judgment_ca", JudgmentType: JudgmentTypeContextApplicability,
		Scope: ScopeProject,
		Subject: JudgmentSubject{SubjectType: "context", MemoryRef: &MemoryRef{
			Scope: ScopeProject, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID,
			Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
		}, TargetContextRef: "context_01K"},
		Source:               JudgmentSource{SourceType: "user", SourceID: "local_user"},
		ContextApplicability: &ContextApplicabilityPayload{Result: "applicable", RequiredConditionIDs: []string{}, EvidenceRefs: []EvidenceRef{}},
		BasisRefs:            []BasisRef{}, CreatedAt: "2026-08-07T00:00:00Z",
	}
	j.ContentSHA256, _ = j.ContentHash()
	if j.BasisContextRefs != nil {
		t.Fatal("legacy fixture unexpectedly enriched")
	}
	b, err := j.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("basis_context_refs")) {
		t.Fatalf("legacy canonical bytes gained basis_context_refs: %s", b)
	}
	// This is the pre-MEM-02E hash locked by the existing eight-subtype
	// compatibility test.
	if j.ContentSHA256 != "sha256_ab8511b2af9aeb3a213ecec58c2f1fedaec7b46ea1d69caa6cff09eb6b925b50" {
		t.Fatalf("legacy context judgment hash changed: %s", j.ContentSHA256)
	}
}

func TestContextApplicabilityBasisSchema(t *testing.T) {
	valid := contextJudgment(t, true, "applicable")
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid enriched judgment: %v", err)
	}
	b1, _ := valid.EncodeCanonical()
	reordered := valid
	reordered.BasisContextRefs = []string{"context_basis_a", "context_basis_b"}
	reordered.ContentSHA256, _ = reordered.ContentHash()
	b2, _ := reordered.EncodeCanonical()
	if !bytes.Equal(b1, b2) {
		t.Fatalf("basis set ordering changed canonical bytes:\n%s\n%s", b1, b2)
	}

	for _, refs := range [][]string{{}, {"../escape"}, {"context_basis_a", "context_basis_a"}} {
		bad := contextJudgment(t, true, "applicable")
		bad.BasisContextRefs = refs
		bad.ContentSHA256, _ = bad.ContentHash()
		if err := bad.Validate(); err == nil {
			t.Fatalf("invalid basis refs accepted: %#v", refs)
		}
	}

	nonContext := validConfirmationJudgment()
	nonContext.BasisContextRefs = []string{"context_basis_a"}
	nonContext.ContentSHA256, _ = nonContext.ContentHash()
	if err := nonContext.Validate(); err == nil {
		t.Fatal("non-context judgment accepted basis_context_refs")
	}
}

func TestContextApplicabilityResultConditionMatrix(t *testing.T) {
	for _, result := range []string{"exact", "applicable", "not_applicable", "unknown"} {
		j := contextJudgment(t, true, result)
		if err := j.Validate(); err != nil {
			t.Fatalf("result %s: %v", result, err)
		}
		j.ContextApplicability.RequiredConditionIDs = []string{"condition_sqlite_driver"}
		j.ContentSHA256, _ = j.ContentHash()
		if err := j.Validate(); err == nil {
			t.Fatalf("result %s accepted condition ids", result)
		}
	}
	conditional := contextJudgment(t, true, "conditionally_applicable")
	if err := conditional.Validate(); err != nil {
		t.Fatal(err)
	}
	conditional.ContextApplicability.RequiredConditionIDs = []string{"condition_sqlite_driver", "condition_sqlite_driver"}
	conditional.ContentSHA256, _ = conditional.ContentHash()
	if err := conditional.Validate(); err == nil {
		t.Fatal("duplicate required condition accepted")
	}
	badResult := contextJudgment(t, true, "unavailable")
	if err := badResult.Validate(); err == nil {
		t.Fatal("persisted unavailable result accepted")
	}
}

func TestContextApplicabilityRejectsDuplicateEvidence(t *testing.T) {
	j := contextJudgment(t, true, "applicable")
	ref := validEvidenceGeneration().EvidenceRefs[0]
	j.ContextApplicability.EvidenceRefs = []EvidenceRef{ref, ref}
	j.ContentSHA256, _ = j.ContentHash()
	if err := j.Validate(); err == nil {
		t.Fatal("duplicate evidence refs accepted")
	}
}

func contextStore(t *testing.T, result string) (*FactStore, JudgmentFact) {
	t.Helper()
	s, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	rev := validRevision()
	put(t, s, rev)
	ev := validEvidenceGeneration()
	put(t, s, ev)
	j := contextJudgment(t, true, result)
	j.ContextApplicability.EvidenceRefs = append([]EvidenceRef(nil), ev.EvidenceRefs...)
	j.ContentSHA256, _ = j.ContentHash()
	put(t, s, j)
	return s, j
}

func TestValidateContextApplicabilityFixture(t *testing.T) {
	s, j := contextStore(t, "conditionally_applicable")
	now := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	got, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ContextApplicabilityVerified || got.Result != "conditionally_applicable" || got.TargetContextRef != "context_target_01" {
		t.Fatalf("unexpected result: %#v", got)
	}
	b1, _ := got.EncodeCanonical()
	got2, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := got2.EncodeCanonical()
	if !bytes.Equal(b1, b2) {
		t.Fatal("derived result is not byte stable")
	}
}

func TestValidateContextApplicabilityAllPersistedResults(t *testing.T) {
	for _, result := range []string{"exact", "applicable", "conditionally_applicable", "not_applicable", "unknown"} {
		t.Run(result, func(t *testing.T) {
			s, j := contextStore(t, result)
			got, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
				Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
				Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != ContextApplicabilityVerified || got.Result != result {
				t.Fatalf("result was reinterpreted: %#v", got)
			}
		})
	}
}

func TestValidateContextApplicabilityLegacyUnavailable(t *testing.T) {
	s, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	j := contextJudgment(t, false, "applicable")
	put(t, s, j)
	got, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ContextApplicabilityUnavailable {
		t.Fatalf("legacy status=%s", got.Status)
	}
}

func TestValidateContextApplicabilityRejectsMissingCondition(t *testing.T) {
	s, j := contextStore(t, "conditionally_applicable")
	// Replace the stored judgment with a validly hashed document referring to
	// a condition that the immutable target revision does not contain.
	j.JudgmentID = "judgment_context_missing_condition"
	j.ContextApplicability.RequiredConditionIDs = []string{"condition_missing"}
	j.ContentSHA256, _ = j.ContentHash()
	put(t, s, j)
	_, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("missing condition code=%s err=%v", ErrorCode(err), err)
	}
}

func TestValidateContextApplicabilityRequiresExplicitNow(t *testing.T) {
	s, j := contextStore(t, "applicable")
	_, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
	})
	if ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero Now code=%s err=%v", ErrorCode(err), err)
	}
}

func TestValidateContextApplicabilityRejectsMemoryRefMismatch(t *testing.T) {
	s, j := contextStore(t, "applicable")
	j.JudgmentID = "judgment_context_wrong_ref"
	j.Subject.MemoryRef.ContentSHA256 = testHash2
	j.ContentSHA256, _ = j.ContentHash()
	put(t, s, j)
	_, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("mismatched MemoryRef accepted")
	}
}

func TestValidateContextApplicabilityAcceptsDoesNotApplyCondition(t *testing.T) {
	s, err := OpenProject(tempRoot(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	rev := validRevision()
	cond := rev.AppliesWhen[0]
	cond.ConditionID = "condition_forbidden_driver"
	rev.DoesNotApplyWhen = []ApplicabilityCondition{cond}
	rev = fillRevisionHash(rev)
	put(t, s, rev)
	j := contextJudgment(t, true, "conditionally_applicable")
	j.Subject.MemoryRef.ContentSHA256 = rev.ContentSHA256
	j.ContextApplicability.RequiredConditionIDs = []string{cond.ConditionID}
	j.ContentSHA256, _ = j.ContentHash()
	put(t, s, j)
	got, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if err != nil || got.Status != ContextApplicabilityVerified {
		t.Fatalf("does_not_apply condition was not accepted: result=%#v err=%v", got, err)
	}
}

func TestValidateContextApplicabilityRejectsMissingEvidence(t *testing.T) {
	s, j := contextStore(t, "applicable")
	j.JudgmentID = "judgment_context_missing_evidence"
	j.ContextApplicability.EvidenceRefs = []EvidenceRef{{
		Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_missing", ContentSHA256: testHash,
	}}
	j.ContentSHA256, _ = j.ContentHash()
	put(t, s, j)
	_, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("missing evidence code=%s err=%v", ErrorCode(err), err)
	}
}

func TestValidateContextApplicabilitySupersedeIdentity(t *testing.T) {
	s, old := contextStore(t, "applicable")
	newer := contextJudgment(t, true, "not_applicable")
	newer.JudgmentID = "judgment_context_new"
	newer.BasisContextRefs = []string{"context_new_basis"} // basis may be revised
	newer.SupersedesJudgmentRef = &JudgmentRef{
		Scope: old.Scope, JudgmentType: old.JudgmentType,
		JudgmentID: old.JudgmentID, ContentSHA256: old.ContentSHA256,
	}
	newer.ContentSHA256, _ = newer.ContentHash()
	put(t, s, newer)
	if _, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: newer.JudgmentID, Store: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("valid supersede chain rejected: %v", err)
	}

	bad := newer
	bad.JudgmentID = "judgment_context_bad_supersede"
	bad.SupersedesJudgmentRef = &JudgmentRef{
		Scope: old.Scope, JudgmentType: old.JudgmentType,
		JudgmentID: old.JudgmentID, ContentSHA256: testHash2,
	}
	bad.ContentSHA256, _ = bad.ContentHash()
	put(t, s, bad)
	_, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
		Scope: ScopeProject, JudgmentID: bad.JudgmentID, Store: s,
		Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("supersede ref hash mismatch accepted")
	}
}

func TestValidateContextApplicabilityZeroWrites(t *testing.T) {
	s, j := contextStore(t, "applicable")
	before := fileCount(t, s.root)
	for i := 0; i < 2; i++ {
		if _, err := ValidateContextApplicability(context.Background(), ContextApplicabilityRequest{
			Scope: ScopeProject, JudgmentID: j.JudgmentID, Store: s,
			Now: time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := fileCount(t, s.root); got != before {
		t.Fatalf("read-only validation wrote files: before=%d after=%d", before, got)
	}
}
