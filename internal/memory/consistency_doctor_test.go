package memory

// MEM-02-07 failure-first tests: consistency Doctor. The doctor is strictly
// read-only: it inspects Judgment → Memory/Outcome/Policy/Generation
// reference integrity, supersede chains, cross-scope references and wrong
// subjects, and emits stable, redacted findings. It never repairs, deletes
// or touches CURRENT.

import (
	"context"
	"strings"
	"testing"
)

func TestConsistencyDoctorCleanStore(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	ev := validEvidenceGeneration()
	putRevisionEvidence(t, s, rev, ev)

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Healthy {
		t.Errorf("clean store must be healthy, got %+v", rep.Findings)
	}
	if len(rep.Findings) != 0 {
		t.Errorf("clean store must have no findings, got %+v", rep.Findings)
	}
}

func TestConsistencyDoctorDetectsRetrievalContextMismatch(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	a := anchoredUsage(t)
	a.UsageID = "usage_context_a"
	h, err := a.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	a.ContentSHA256 = h
	if _, err := s.Put(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	b := anchoredUsage(t)
	b.UsageID = "usage_context_b"
	b.MemoryContext.ProjectGenerationRef.GenerationID = "gen_project_000011"
	h, err = b.ContentHash()
	if err != nil {
		t.Fatal(err)
	}
	b.ContentSHA256 = h
	if _, err := s.Put(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	report, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if report.Healthy || !hasFinding(report, findingUsageContextMismatch) {
		t.Fatalf("expected retrieval context mismatch finding, got %+v", report.Findings)
	}
}

func TestConsistencyDoctorAttributionOverrideMismatch(t *testing.T) {
	store, receipt, _ := attributionFixture(t, "evaluated")
	if _, err := CommitOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt}); err != nil {
		t.Fatal(err)
	}
	outcomes, err := BuildOutcomes(context.Background(), AttributionRequest{Store: store, Receipt: receipt})
	if err != nil {
		t.Fatal(err)
	}
	j := JudgmentFact{SchemaVersion: 1, JudgmentID: "judgment_bad_override", JudgmentType: JudgmentTypeAttributionOverride, Scope: ScopeProject, Subject: JudgmentSubject{SubjectType: "memory_outcome", OutcomeID: outcomes[0].OutcomeID}, Source: JudgmentSource{SourceType: "local_user", SourceID: "user_1"}, AttributionOverride: &AttributionOverridePayload{PreviousEffect: "harmed", NewEffect: "neutral", Reason: "bad fixture"}, CreatedAt: "2026-08-13T00:00:00Z"}
	j = fillJudgmentHash(j)
	put(t, store, j)
	rep, err := CheckConsistency(context.Background(), store, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Healthy || !hasFinding(rep, findingAttributionMismatch) {
		t.Fatalf("expected attribution mismatch finding: %+v", rep.Findings)
	}
}

func TestConsistencyDoctorOrphanMemoryRef(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// Judgment whose subject points at a revision that does not exist.
	j := validConfirmationJudgment()
	j.JudgmentID = "judgment_orphan_mem"
	j.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &MemoryRef{
		Scope: ScopeProject, MemoryType: MemoryTypeStrategy,
		MemoryID: "mem_does_not_exist", Revision: 1, ContentSHA256: testHash,
	}}
	j = fillJudgmentHash(j)
	put(t, s, j)

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Healthy {
		t.Error("orphan memory ref must make the store unhealthy")
	}
	if !hasFinding(rep, "orphan_memory_ref") {
		t.Errorf("missing orphan_memory_ref finding: %+v", rep.Findings)
	}
}

func TestConsistencyDoctorCrossScopeReference(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	// Judgment in project scope whose subject memory_ref claims global scope.
	j := validConfirmationJudgment()
	j.JudgmentID = "judgment_cross_scope"
	j.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &MemoryRef{
		Scope: ScopeGlobal, MemoryType: MemoryTypeStrategy,
		MemoryID: "mem_global_target", Revision: 1, ContentSHA256: testHash,
	}}
	j = fillJudgmentHash(j)
	put(t, s, j)

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(rep, "cross_scope_reference") {
		t.Errorf("missing cross_scope_reference finding: %+v", rep.Findings)
	}
}

func TestConsistencyDoctorSupersedeCycle(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	put(t, s, rev)

	mk := func(id string, supersedes *JudgmentRef) JudgmentFact {
		j := validConfirmationJudgment()
		j.JudgmentID = id
		j.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &MemoryRef{
			Scope: rev.Scope, MemoryType: rev.MemoryType,
			MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
		}}
		if supersedes != nil {
			j.SupersedesJudgmentRef = supersedes
		}
		j = fillJudgmentHash(j)
		put(t, s, j)
		return j
	}
	// Closed cycle c1 -> c3 -> c2 -> c1. Supersede targets are not
	// existence-checked by the schema, so the refs can carry stable
	// placeholder hashes; the doctor must detect the cycle instead of
	// looping forever.
	mk("judgment_cycle_1", &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_cycle_3", ContentSHA256: testHash})
	mk("judgment_cycle_2", &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_cycle_1", ContentSHA256: testHash})
	mk("judgment_cycle_3", &JudgmentRef{Scope: rev.Scope, JudgmentType: JudgmentTypeConfirmation, JudgmentID: "judgment_cycle_2", ContentSHA256: testHash})

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(rep, "supersede_cycle") {
		t.Errorf("missing supersede_cycle finding: %+v", rep.Findings)
	}
}

func TestConsistencyDoctorOrphanPolicyRef(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	put(t, s, rev)

	j := validFreshnessJudgmentFor(rev, "fresh", "2026-08-11T00:00:00Z")
	j.JudgmentID = "judgment_orphan_policy"
	j.FreshnessEvaluation.FreshnessPolicyRef = PolicyRef{
		PolicyID: "policy_missing", PolicyType: PolicyTypeFreshness, ContentSHA256: testHash,
	}
	j = fillJudgmentHash(j)
	put(t, s, j)

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(rep, "orphan_policy_ref") {
		t.Errorf("missing orphan_policy_ref finding: %+v", rep.Findings)
	}
}

func TestConsistencyDoctorSupersedeTargetTypeMismatch(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	put(t, s, rev)

	// A confirmation judgment that supersedes an actual freshness judgment
	// (registered type differs) must be flagged even if the ref lies about
	// its type.
	old := validFreshnessJudgmentFor(rev, "fresh", "2026-08-11T00:00:00Z")
	old.JudgmentID = "judgment_target_freshness"
	old = fillJudgmentHash(old)
	put(t, s, old)

	j := validConfirmationJudgment()
	j.JudgmentID = "judgment_lying_supersede"
	j.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &MemoryRef{
		Scope: rev.Scope, MemoryType: rev.MemoryType,
		MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
	}}
	j.SupersedesJudgmentRef = &JudgmentRef{
		Scope: rev.Scope, JudgmentType: JudgmentTypeConfirmation, // lying declaration
		JudgmentID: old.JudgmentID, ContentSHA256: old.ContentSHA256,
	}
	j = fillJudgmentHash(j)
	put(t, s, j)

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(rep, "subject_payload_mismatch") {
		t.Errorf("lying supersede type must be flagged against the actual target type: %+v", rep.Findings)
	}
}

func TestConsistencyDoctorSubjectPayloadMismatch(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	put(t, s, rev)
	// freshness judgment whose subject memory revision differs from the
	// payload memory_ref revision.
	j := validFreshnessJudgmentFor(rev, "fresh", "2026-08-11T00:00:00Z")
	j.JudgmentID = "judgment_subject_mismatch"
	other := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: 1, ContentSHA256: testHash}
	j.Subject.MemoryRef = &other
	j = fillJudgmentHash(j)
	put(t, s, j)

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(rep, "subject_payload_mismatch") {
		t.Errorf("missing subject_payload_mismatch finding: %+v", rep.Findings)
	}
}

func TestConsistencyDoctorCorruptFactAndRedaction(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	put(t, s, rev)

	// Plant a corrupt judgment file.
	if _, err := plantRawFact(root, FactKindJudgment, "judgment_corrupt", []byte("{not json")); err != nil {
		t.Fatal(err)
	}

	rep, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(rep, "corrupt_fact") {
		t.Errorf("missing corrupt_fact finding: %+v", rep.Findings)
	}
	// The report must never leak absolute paths or raw validator text with
	// commands/prompts.
	raw, err := rep.EncodeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), root) {
		t.Error("report must not leak the store root path")
	}
	for _, f := range rep.Findings {
		if strings.Contains(f.Detail, "\n") || strings.Contains(f.Detail, "/Users/") || strings.Contains(f.Detail, "BEGIN") {
			t.Errorf("finding detail not redacted: %+v", f)
		}
	}
}

func TestConsistencyDoctorReadOnly(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validRevision()
	put(t, s, rev)
	j := validConfirmationJudgment()
	j.JudgmentID = "judgment_readonly"
	j.Subject = JudgmentSubject{SubjectType: "memory_revision", MemoryRef: &MemoryRef{
		Scope: rev.Scope, MemoryType: rev.MemoryType,
		MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
	}}
	j = fillJudgmentHash(j)
	put(t, s, j)

	before, err := s.List(context.Background(), FactKindJudgment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject}); err != nil {
		t.Fatal(err)
	}
	after, err := s.List(context.Background(), FactKindJudgment)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Error("doctor must not write or remove facts")
	}
}

func TestConsistencyDoctorScopeIsolation(t *testing.T) {
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	if _, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeGlobal}); ErrorCode(err) != CodeScopeMismatch {
		t.Fatalf("cross-scope doctor must fail closed, got %v", err)
	}
}

// ---- helpers ----

func hasFinding(rep *ConsistencyReport, code string) bool {
	for _, f := range rep.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestConsistencyDoctorConflictReviewReferences(t *testing.T) {
	s := openProject(t, tempRoot(t), Options{})
	rev := validEvidenceValidatedRevision()
	put(t, s, rev)
	j := validConflictJudgment(rev, "conflict")
	j.ConflictReview.CounterpartMemoryRefs = []MemoryRef{{
		Scope: ScopeProject, MemoryType: MemoryTypePattern, MemoryID: "mem_missing_counterpart",
		Revision: 1, ContentSHA256: testHash,
	}}
	j.ConflictReview.EvidenceRefs = []EvidenceRef{{
		Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_missing", ContentSHA256: testHash,
	}}
	j.ContentSHA256 = ""
	j = fillJudgmentHash(j)
	put(t, s, j)
	report, err := CheckConsistency(context.Background(), s, ConsistencyRequest{Scope: ScopeProject})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(report, findingOrphanMemoryRef) || !hasFinding(report, findingOrphanEvidenceRef) {
		t.Fatalf("doctor must report conflict reference gaps: %+v", report.Findings)
	}
}

func validFreshnessJudgmentFor(rev MemoryRevision, result, evaluatedAt string) JudgmentFact {
	return JudgmentFact{
		SchemaVersion: 1,
		JudgmentID:    "judgment_freshness_helper",
		JudgmentType:  JudgmentTypeFreshnessEvaluation,
		Scope:         rev.Scope,
		Subject: JudgmentSubject{
			SubjectType: "memory_revision",
			MemoryRef: &MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
		},
		Source: JudgmentSource{SourceType: "fixture_oracle", SourceID: "fixture_doc"},
		FreshnessEvaluation: &FreshnessEvaluationPayload{
			MemoryRef: MemoryRef{
				Scope: rev.Scope, MemoryType: rev.MemoryType,
				MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256,
			},
			Result:      result,
			EvaluatedAt: evaluatedAt,
			FreshnessPolicyRef: PolicyRef{
				PolicyID: "freshness_policy_v1", PolicyType: PolicyTypeFreshness, ContentSHA256: testHash,
			},
		},
		CreatedAt: evaluatedAt,
	}
}
