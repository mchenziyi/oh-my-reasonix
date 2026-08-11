package memory

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func removeRevalJudgment(t *testing.T, s *FactStore, id string) {
	t.Helper()
	if err := os.Remove(filepath.Join(s.root, "facts", "judgments", id+".json")); err != nil {
		t.Fatal(err)
	}
}

func putRevalEvidence(t *testing.T, s *FactStore, rev MemoryRevision, generation int, evidenceType, createdAt string) MemoryEvidenceGeneration {
	t.Helper()
	ev := validEvidenceGeneration()
	ev.MemoryID = rev.MemoryID
	ev.Revision = rev.Revision
	ev.EvidenceGeneration = generation
	ev.EvidenceRefs = []EvidenceRef{{
		Scope: ScopeProject, EvidenceType: evidenceType,
		EvidenceID: "evidence_reval", ContentSHA256: testHash,
	}}
	ev.CreatedAt = createdAt
	ev = fillEvidenceHash(ev)
	put(t, s, ev)
	return ev
}

func evaluateOne(t *testing.T, s *FactStore, now time.Time, ref PolicyRef) (*RevalidationReport, RevalidationCandidate) {
	t.Helper()
	rep, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: now, FreshnessPolicyRef: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(rep.Candidates))
	}
	return rep, rep.Candidates[0]
}

func hasRevalidationDiagnostic(rep *RevalidationReport, code string) bool {
	for _, d := range rep.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}

func TestRevalidationRejectsSubjectPayloadIdentityMismatch(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_identity", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	j := putFreshnessJudgment(t, s, "judgment_reval_identity", rev, "fresh",
		revalNow().AddDate(0, 0, -1).Format(time.RFC3339), ref)

	// Replace the valid fact with one whose payload points at another valid
	// MemoryRef identity. The envelope itself remains schema-valid.
	removeRevalJudgment(t, s, j.JudgmentID)
	j.FreshnessEvaluation.MemoryRef.MemoryType = MemoryTypePattern
	j = fillJudgmentHash(j)
	put(t, s, j)
	if _, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
	}); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("subject/payload identity mismatch must fail closed, got %v", err)
	}
}

func TestRevalidationRejectsMatchingButForgedMemoryRefs(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_forged_ref", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	j := putFreshnessJudgment(t, s, "judgment_reval_forged_ref", rev, "fresh",
		revalNow().AddDate(0, 0, -1).Format(time.RFC3339), ref)
	removeRevalJudgment(t, s, j.JudgmentID)
	j.Subject.MemoryRef.ContentSHA256 = testHash
	j.FreshnessEvaluation.MemoryRef.ContentSHA256 = testHash
	j = fillJudgmentHash(j)
	put(t, s, j)

	if _, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
	}); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("forged refs for the same revision identity must fail closed, got %v", err)
	}
}

func TestRevalidationRejectsForgedSupersedeReference(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_supersede", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	old := putFreshnessJudgment(t, s, "judgment_reval_supersede_old", rev, "fresh",
		revalNow().AddDate(0, 0, -2).Format(time.RFC3339), ref)
	newJ := putFreshnessJudgment(t, s, "judgment_reval_supersede_new", rev, "aging",
		revalNow().AddDate(0, 0, -1).Format(time.RFC3339), ref)
	removeRevalJudgment(t, s, newJ.JudgmentID)
	newJ.SupersedesJudgmentRef = &JudgmentRef{
		Scope: rev.Scope, JudgmentType: JudgmentTypeFreshnessEvaluation,
		JudgmentID: old.JudgmentID, ContentSHA256: testHash,
	}
	newJ = fillJudgmentHash(newJ)
	put(t, s, newJ)

	if _, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
	}); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("forged supersede ref must fail closed, got %v", err)
	}
}

func TestRevalidationExpiredJudgmentFallsBackToWindow(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_expired", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	putFreshnessJudgment(t, s, "judgment_reval_expired", rev, "fresh",
		revalNow().AddDate(0, 0, -100).Format(time.RFC3339), ref)

	rep, cand := evaluateOne(t, s, revalNow(), ref)
	if cand.Result != RevalidationNeedsRevalidation || cand.Reason != "stale_window" {
		t.Fatalf("expired judgment must fall back to stale window, got %+v", cand)
	}
	if !hasRevalidationDiagnostic(rep, "evaluation_expired") {
		t.Fatalf("expired judgment must be diagnosed, got %+v", rep.Diagnostics)
	}
}

func TestRevalidationEvidenceTypeControlsActivity(t *testing.T) {
	for _, tc := range []struct {
		name         string
		evidenceType string
		want         RevalidationResult
	}{
		{name: "unrelated", evidenceType: "episode", want: RevalidationNeedsRevalidation},
		{name: "allowed", evidenceType: "test_result", want: RevalidationFresh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openProject(t, tempRoot(t), Options{})
			ref := putRevalFreshnessPolicy(t, s)
			rev := putRevalRevision(t, s, "mem_reval_evidence", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
			putRevalEvidence(t, s, rev, 1, tc.evidenceType, revalNow().AddDate(0, 0, -1).Format(time.RFC3339))
			_, cand := evaluateOne(t, s, revalNow(), ref)
			if cand.Result != tc.want {
				t.Fatalf("evidence type %q produced %s, want %s", tc.evidenceType, cand.Result, tc.want)
			}
		})
	}
}

func TestRevalidationFutureEvidenceIsDiagnosedAndIgnored(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_future_evidence", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	putRevalEvidence(t, s, rev, 1, "test_result", revalNow().AddDate(0, 0, 1).Format(time.RFC3339))

	rep, cand := evaluateOne(t, s, revalNow(), ref)
	if cand.Result != RevalidationNeedsRevalidation || cand.Reason != "stale_window" {
		t.Fatalf("future evidence must not refresh activity, got %+v", cand)
	}
	if !hasRevalidationDiagnostic(rep, "future_evidence") {
		t.Fatalf("future evidence must be diagnosed, got %+v", rep.Diagnostics)
	}
}

func TestRevalidationRejectsFuturePolicyAndRevision(t *testing.T) {
	t.Run("policy", func(t *testing.T) {
		s := openProject(t, tempRoot(t), Options{})
		ref := putRevalFreshnessPolicy(t, s)
		if _, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
			Scope: ScopeProject, Now: revalNow().Add(-time.Hour), FreshnessPolicyRef: ref,
		}); ErrorCode(err) != CodeEvaluationFutureReference {
			t.Fatalf("future policy must fail closed, got %v", err)
		}
	})

	t.Run("revision", func(t *testing.T) {
		s := openProject(t, tempRoot(t), Options{})
		ref := putRevalFreshnessPolicy(t, s)
		putRevalRevision(t, s, "mem_reval_future_revision", revalNow().AddDate(0, 0, 1).Format(time.RFC3339))
		if _, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
			Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
		}); ErrorCode(err) != CodeEvaluationFutureReference {
			t.Fatalf("future revision must fail closed, got %v", err)
		}
	})
}

func TestRevalidationRejectsOrphanBasisRef(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_basis", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	j := putFreshnessJudgment(t, s, "judgment_reval_basis", rev, "fresh",
		revalNow().AddDate(0, 0, -1).Format(time.RFC3339), ref)
	removeRevalJudgment(t, s, j.JudgmentID)
	orphan := EvidenceRef{Scope: ScopeProject, EvidenceType: "test_result", EvidenceID: "evidence_missing", ContentSHA256: testHash}
	j.FreshnessEvaluation.BasisRefs = []BasisRef{{EvidenceRef: &orphan}}
	j = fillJudgmentHash(j)
	put(t, s, j)

	if _, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
	}); ErrorCode(err) != CodeSchemaInvalid {
		t.Fatalf("orphan basis ref must fail closed, got %v", err)
	}
}

func TestRevalidationConflictingTerminalsFallBack(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_conflict", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	putFreshnessJudgment(t, s, "judgment_reval_conflict_a", rev, "fresh",
		revalNow().AddDate(0, 0, -2).Format(time.RFC3339), ref)
	putFreshnessJudgment(t, s, "judgment_reval_conflict_b", rev, "aging",
		revalNow().AddDate(0, 0, -1).Format(time.RFC3339), ref)

	rep, cand := evaluateOne(t, s, revalNow(), ref)
	if cand.Result != RevalidationNeedsRevalidation {
		t.Fatalf("conflicting terminals must use window fallback, got %+v", cand)
	}
	if !hasRevalidationDiagnostic(rep, "conflicting_freshness_judgments") {
		t.Fatalf("terminal conflict must be diagnosed, got %+v", rep.Diagnostics)
	}
}

func TestRevalidationConsistentTerminalsAreDeterministic(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	ref := putRevalFreshnessPolicy(t, s)
	rev := putRevalRevision(t, s, "mem_reval_consistent", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	putFreshnessJudgment(t, s, "judgment_reval_consistent_b", rev, "aging",
		revalNow().AddDate(0, 0, -1).Format(time.RFC3339), ref)
	putFreshnessJudgment(t, s, "judgment_reval_consistent_a", rev, "aging",
		revalNow().AddDate(0, 0, -2).Format(time.RFC3339), ref)

	rep1, cand1 := evaluateOne(t, s, revalNow(), ref)
	rep2, cand2 := evaluateOne(t, s, revalNow(), ref)
	if cand1.Result != RevalidationAging || cand1.Reason != "judgment_driven" {
		t.Fatalf("consistent terminals should be adopted, got %+v", cand1)
	}
	b1, err := rep1.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	b2, err := rep2.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) || cand1 != cand2 {
		t.Fatal("consistent terminal evaluation must be byte-stable")
	}
}
