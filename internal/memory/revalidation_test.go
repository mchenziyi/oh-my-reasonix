package memory

// MEM-02-06 failure-first tests: Revalidation evaluation. The evaluator is
// read-only: it derives fresh|aging|needs_revalidation candidates from the
// freshness_evaluation judgments and the frozen freshness policy time
// windows, never writes a fact, never mutates a Revision and never lets time
// produce frozen/superseded/archived. Diagnostics surface policy drift,
// future evaluated_at and hash drift; identical inputs yield byte-stable
// output.

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func revalNow() time.Time {
	return time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)
}

func putRevalRevision(t *testing.T, s *FactStore, id string, createdAt string) MemoryRevision {
	t.Helper()
	rev := validRevision()
	rev.MemoryID = id
	rev.CanonicalKey = "reval-" + id
	rev.Title = "Reval " + id
	rev.CreatedAt = createdAt
	rev = fillRevisionHash(rev)
	put(t, s, rev)
	return rev
}

func putRevalFreshnessPolicy(t *testing.T, s *FactStore) PolicyRef {
	t.Helper()
	p := policyOf(PolicyTypeFreshness)
	if _, err := s.Put(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return PolicyRef{
		PolicyID: p.PolicyID, PolicyType: PolicyTypeFreshness, ContentSHA256: p.ContentSHA256,
	}
}

func putFreshnessJudgment(t *testing.T, s *FactStore, id string, rev MemoryRevision, result string, evaluatedAt string, policyRef PolicyRef) JudgmentFact {
	t.Helper()
	j := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    id,
		JudgmentType:  JudgmentTypeFreshnessEvaluation,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_reval"},
		FreshnessEvaluation: &FreshnessEvaluationPayload{
			MemoryRef: MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
			Result:             result,
			EvaluatedAt:        evaluatedAt,
			FreshnessPolicyRef: policyRef,
		},
		CreatedAt: evaluatedAt,
	}
	j = fillJudgmentHash(j)
	put(t, s, j)
	return j
}

func TestRevalidationWindowDriven(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ref := putRevalFreshnessPolicy(t, s)

	now := revalNow()
	fresh := putRevalRevision(t, s, "mem_reval_fresh", now.AddDate(0, 0, -10).Format(time.RFC3339))
	aging := putRevalRevision(t, s, "mem_reval_aging", now.AddDate(0, 0, -120).Format(time.RFC3339))
	stale := putRevalRevision(t, s, "mem_reval_stale", now.AddDate(0, 0, -400).Format(time.RFC3339))

	rep, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: now, FreshnessPolicyRef: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]RevalidationResult{}
	for _, c := range rep.Candidates {
		got[c.MemoryID] = c.Result
	}
	if got[fresh.MemoryID] != RevalidationFresh {
		t.Errorf("%s = %s, want fresh", fresh.MemoryID, got[fresh.MemoryID])
	}
	if got[aging.MemoryID] != RevalidationAging {
		t.Errorf("%s = %s, want aging", aging.MemoryID, got[aging.MemoryID])
	}
	if got[stale.MemoryID] != RevalidationNeedsRevalidation {
		t.Errorf("%s = %s, want needs_revalidation", stale.MemoryID, got[stale.MemoryID])
	}
	if len(rep.Diagnostics) != 0 {
		t.Errorf("clean run must have no diagnostics, got %+v", rep.Diagnostics)
	}
}

func TestRevalidationJudgmentDriven(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ref := putRevalFreshnessPolicy(t, s)

	rev := putRevalRevision(t, s, "mem_reval_judged", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	// Old judgment is superseded by a newer one: the newest live judgment
	// (fresh, -1d) must win over the superseded aging one.
	old := putFreshnessJudgment(t, s, "judgment_reval_old", rev, "aging",
		revalNow().AddDate(0, 0, -30).Format(time.RFC3339), ref)
	newJ := JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_reval_new",
		JudgmentType:  JudgmentTypeFreshnessEvaluation,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_reval"},
		FreshnessEvaluation: &FreshnessEvaluationPayload{
			MemoryRef: MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
			Result:             "fresh",
			EvaluatedAt:        revalNow().AddDate(0, 0, -1).Format(time.RFC3339),
			FreshnessPolicyRef: ref,
		},
		SupersedesJudgmentRef: &JudgmentRef{
			Scope: rev.Scope, JudgmentType: JudgmentTypeFreshnessEvaluation,
			JudgmentID: old.JudgmentID, ContentSHA256: old.ContentSHA256,
		},
		CreatedAt: revalNow().AddDate(0, 0, -1).Format(time.RFC3339),
	}
	newJ = fillJudgmentHash(newJ)
	put(t, s, newJ)

	rep, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(rep.Candidates))
	}
	if rep.Candidates[0].Result != RevalidationFresh {
		t.Errorf("result = %s, want fresh (newest live judgment supersedes old)", rep.Candidates[0].Result)
	}
}

func TestRevalidationDiagnostics(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ref := putRevalFreshnessPolicy(t, s)
	now := revalNow()

	// Future evaluated_at must not be adopted; it is surfaced as a
	// diagnostic and the window drives the result.
	futureRev := putRevalRevision(t, s, "mem_reval_future", now.AddDate(0, 0, -10).Format(time.RFC3339))
	putFreshnessJudgment(t, s, "judgment_reval_future", futureRev, "needs_revalidation",
		now.AddDate(0, 0, 30).Format(time.RFC3339), ref)

	// Policy drift: the judgment cites a different policy; it must not be
	// adopted either.
	otherRef := ref
	otherRef.PolicyID = "freshness_policy_other"
	otherRef.ContentSHA256 = testHash
	driftRev := putRevalRevision(t, s, "mem_reval_drift", now.AddDate(0, 0, -10).Format(time.RFC3339))
	putFreshnessJudgment(t, s, "judgment_reval_drift", driftRev, "needs_revalidation",
		now.AddDate(0, 0, -1).Format(time.RFC3339), otherRef)

	rep, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: now, FreshnessPolicyRef: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, d := range rep.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["future_evaluated_at"] {
		t.Errorf("future evaluated_at must be diagnosed, got %+v", rep.Diagnostics)
	}
	if !codes["policy_drift"] {
		t.Errorf("policy drift must be diagnosed, got %+v", rep.Diagnostics)
	}
	// Both memories still get window-driven results (fresh), never frozen.
	for _, c := range rep.Candidates {
		if c.Result == RevalidationNeedsRevalidation && c.MemoryID != "mem_reval_stale" {
			// (window result for 10-day-old memories is fresh)
		}
		if c.Result != RevalidationFresh {
			t.Errorf("%s = %s, want fresh (window fallback)", c.MemoryID, c.Result)
		}
	}
}

func TestRevalidationDeterministicAndReadOnly(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ref := putRevalFreshnessPolicy(t, s)
	putRevalRevision(t, s, "mem_reval_a", revalNow().AddDate(0, 0, -400).Format(time.RFC3339))
	putRevalRevision(t, s, "mem_reval_b", revalNow().AddDate(0, 0, -5).Format(time.RFC3339))

	before, err := s.List(context.Background(), FactKindMemoryRevision)
	if err != nil {
		t.Fatal(err)
	}
	rep1, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	rep2, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	b1, _ := json.Marshal(rep1)
	b2, _ := json.Marshal(rep2)
	if string(b1) != string(b2) {
		t.Error("revalidation report must be byte-stable for identical inputs")
	}
	after, err := s.List(context.Background(), FactKindMemoryRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Error("revalidation must not write or remove facts")
	}
	if len(rep1.Candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(rep1.Candidates))
	}
}

func TestRevalidationPolicyHashMismatchFailClosed(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	putRevalFreshnessPolicy(t, s)
	putRevalRevision(t, s, "mem_reval_pol", revalNow().AddDate(0, 0, -10).Format(time.RFC3339))

	badRef := PolicyRef{PolicyID: "policy_freshness", PolicyType: PolicyTypeFreshness, ContentSHA256: testHash}
	_, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeProject, Now: revalNow(), FreshnessPolicyRef: badRef,
	})
	if ErrorCode(err) != CodeHashMismatch && ErrorCode(err) != CodeNotFound {
		t.Fatalf("policy hash drift must fail closed, got %v", err)
	}
}

func TestRevalidationScopeIsolation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	ref := putRevalFreshnessPolicy(t, s)
	if _, err := EvaluateRevalidation(context.Background(), s, RevalidationRequest{
		Scope: ScopeGlobal, Now: revalNow(), FreshnessPolicyRef: ref,
	}); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("cross-scope revalidation must fail closed, got %v", err)
	}
}
