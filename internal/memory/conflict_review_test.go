package memory

import (
	"bytes"
	"testing"
)

func validConflictJudgment(rev MemoryRevision, result string) JudgmentFact {
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_conflict_01",
		JudgmentType:  JudgmentTypeConflictReview,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID,
				Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_001"},
		ConflictReview: &ConflictReviewPayload{
			Result: result, EvaluationScope: "generation_full_scan",
			MemoryContext: validCriticContext(), CounterpartMemoryRefs: []MemoryRef{}, EvidenceRefs: []EvidenceRef{},
		},
		BasisRefs: []BasisRef{}, CreatedAt: "2026-08-11T00:00:00Z",
	}
	if result == "conflict" {
		other := validRevision()
		other.MemoryID = "mem_conflict_other"
		other.CanonicalKey = "conflict-other"
		other = fillRevisionHash(other)
		j.ConflictReview.CounterpartMemoryRefs = []MemoryRef{{
			Scope: other.Scope, MemoryType: other.MemoryType, MemoryID: other.MemoryID,
			Revision: other.Revision, ContentSHA256: other.ContentSHA256,
		}}
	}
	return fillJudgmentHash(j)
}

func TestConflictReviewRoundTrip(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	for _, result := range []string{"clear", "conflict", "unavailable"} {
		j := validConflictJudgment(rev, result)
		if err := j.Validate(); err != nil {
			t.Fatalf("%s rejected: %v", result, err)
		}
		raw, err := j.EncodeCanonical()
		if err != nil {
			t.Fatal(err)
		}
		back, err := DecodeStrict[JudgmentFact](raw)
		if err != nil {
			t.Fatal(err)
		}
		if back.ConflictReview == nil || back.ConflictReview.Result != result {
			t.Fatalf("conflict_review %s lost in round trip", result)
		}
	}
}

func TestConflictReviewResultCounterpartMatrix(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	clear := validConflictJudgment(rev, "clear")
	other := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: "mem_other", Revision: 1, ContentSHA256: testHash}
	clear.ConflictReview.CounterpartMemoryRefs = []MemoryRef{other}
	if err := clear.Validate(); err == nil {
		t.Fatal("clear with counterpart must be rejected")
	}
	conflict := validConflictJudgment(rev, "conflict")
	conflict.ConflictReview.CounterpartMemoryRefs = nil
	if err := conflict.Validate(); err == nil {
		t.Fatal("conflict without counterpart must be rejected")
	}
}

func TestConflictReviewCanonicalOrderStable(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	a := validConflictJudgment(rev, "conflict")
	first := a.ConflictReview.CounterpartMemoryRefs[0]
	second := first
	second.MemoryID = "mem_conflict_second"
	a.ConflictReview.CounterpartMemoryRefs = []MemoryRef{second, first}
	a = fillJudgmentHash(a)
	b := a
	b.ConflictReview = &ConflictReviewPayload{
		Result: a.ConflictReview.Result, EvaluationScope: a.ConflictReview.EvaluationScope,
		MemoryContext:         a.ConflictReview.MemoryContext,
		CounterpartMemoryRefs: []MemoryRef{first, second}, EvidenceRefs: []EvidenceRef{},
	}
	b.ContentSHA256 = ""
	b = fillJudgmentHash(b)
	ab, _ := a.EncodeCanonical()
	bb, _ := b.EncodeCanonical()
	if !bytes.Equal(ab, bb) || a.ContentSHA256 != b.ContentSHA256 {
		t.Fatal("reference order must not change canonical bytes or hash")
	}
}

func TestExistingCriticReviewGoldenUnchangedByConflictUnion(t *testing.T) {
	j := fillCriticHash(validCriticJudgment(validEvidenceValidatedRevision()))
	const want = "sha256_b121c8acd30e69635853619b04efdbf69a80d2aade0d8e3405d9c9eef42c6fe2"
	if j.ContentSHA256 != want {
		t.Fatalf("critic golden changed: got %s want %s", j.ContentSHA256, want)
	}
}

func TestConflictReviewStrictConstraints(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	other := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: "mem_other", Revision: 1, ContentSHA256: testHash}
	cases := []struct {
		name string
		mut  func(*JudgmentFact)
	}{
		{"unknown result", func(j *JudgmentFact) { j.ConflictReview.Result = "maybe" }},
		{"unknown source", func(j *JudgmentFact) { j.Source.SourceType = "model_guess" }},
		{"wrong subject", func(j *JudgmentFact) { j.Subject.SubjectType = "evidence" }},
		{"self counterpart", func(j *JudgmentFact) { j.ConflictReview.CounterpartMemoryRefs = []MemoryRef{*j.Subject.MemoryRef} }},
		{"duplicate counterpart", func(j *JudgmentFact) { j.ConflictReview.CounterpartMemoryRefs = []MemoryRef{other, other} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			j := validConflictJudgment(rev, "conflict")
			tc.mut(&j)
			if err := j.Validate(); err == nil {
				t.Fatal("invalid conflict_review must be rejected")
			}
		})
	}
}

func TestConflictReviewClearSupersedeNeedsNonPolicyBasis(t *testing.T) {
	rev := validEvidenceValidatedRevision()
	j := validConflictJudgment(rev, "clear")
	j.SupersedesJudgmentRef = &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeConflictReview, JudgmentID: "judgment_old", ContentSHA256: testHash}
	policy := PolicyRef{PolicyID: "policy_only", PolicyType: PolicyTypeTrust, ContentSHA256: testHash}
	j.BasisRefs = []BasisRef{{PolicyRef: &policy}}
	if err := j.Validate(); err == nil {
		t.Fatal("policy-only basis must not prove conflict resolution")
	}
}
