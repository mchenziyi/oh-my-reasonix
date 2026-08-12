package memory

import (
	"context"
	"testing"
	"time"
)

func conflictWorld(t *testing.T, key string) (*FactStore, *GenerationTx, MemoryRevision, MemoryRef) {
	t.Helper()
	root := tempRoot(t)
	s := openProject(t, root, Options{})
	rev := validEvidenceValidatedRevision()
	ev := MemoryEvidenceGeneration{
		SchemaVersion: 1, MemoryID: rev.MemoryID, Revision: rev.Revision, EvidenceGeneration: 1,
		EvidenceRefs:  []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_001", ContentSHA256: testHash}},
		TransactionID: "tx_conflict_target", CreatedAt: "2026-08-11T00:00:00Z",
	}
	ev = fillEvidenceHash(ev)
	other := validRevision()
	other.MemoryID = "mem_conflict_other"
	other.CanonicalKey = "conflict-other"
	other = fillRevisionHash(other)
	otherEvidence := validEvidenceGeneration()
	otherEvidence.MemoryID = other.MemoryID
	otherEvidence.Revision = other.Revision
	otherEvidence.EvidenceRefs = []EvidenceRef{{Scope: ScopeProject, EvidenceType: "episode", EvidenceID: "episode_other", ContentSHA256: testHash}}
	otherEvidence = fillEvidenceHash(otherEvidence)
	putRevisionEvidence(t, s, rev, ev)
	putRevisionEvidence(t, s, other, otherEvidence)
	compiled, err := CompileOKF(context.Background(), s, OKFCompileRequest{
		Scope: ScopeProject,
		Revisions: []MemoryRevisionRef{
			{MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256},
			{MemoryID: other.MemoryID, Revision: other.Revision, ContentSHA256: other.ContentSHA256},
		},
		Evidence: []MemoryEvidenceRef{
			{MemoryID: ev.MemoryID, Revision: ev.Revision, EvidenceGeneration: ev.EvidenceGeneration, EvidenceSetSHA256: ev.EvidenceSetSHA256},
			{MemoryID: otherEvidence.MemoryID, Revision: otherEvidence.Revision, EvidenceGeneration: otherEvidence.EvidenceGeneration, EvidenceSetSHA256: otherEvidence.EvidenceSetSHA256},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	gs := NewGenerationStore(s)
	begin := beginReq(key, nil)
	begin.CompilerVersion = OKFCompilerVersion
	begin.CanonicalizationVersion = OKFCanonicalizationVersion
	tx, err := gs.Begin(context.Background(), begin)
	if err != nil {
		t.Fatal(err)
	}
	if err := gs.PrepareManifest(context.Background(), tx, manifestFor(tx, compiled.Inputs)); err != nil {
		t.Fatal(err)
	}
	if err := gs.WriteCompiledOutput(context.Background(), tx, compiled.Outputs); err != nil {
		t.Fatal(err)
	}
	if err := gs.ValidateStaging(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	if _, err := gs.Commit(context.Background(), tx); err != nil {
		t.Fatal(err)
	}
	ref := MemoryRef{Scope: other.Scope, MemoryType: other.MemoryType, MemoryID: other.MemoryID, Revision: other.Revision, ContentSHA256: other.ContentSHA256}
	return s, tx, rev, ref
}

func putConflictReview(t *testing.T, s *FactStore, rev MemoryRevision, mc MemoryContext, id, result, scope string, supersedes *JudgmentRef, counterparts []MemoryRef) JudgmentFact {
	t.Helper()
	j := validConflictJudgment(rev, result)
	j.JudgmentID = id
	j.ConflictReview.MemoryContext = mc
	j.ConflictReview.EvaluationScope = scope
	j.ConflictReview.CounterpartMemoryRefs = counterparts
	j.SupersedesJudgmentRef = supersedes
	if result == "clear" && supersedes != nil {
		basis := MemoryRef{Scope: rev.Scope, MemoryType: rev.MemoryType, MemoryID: rev.MemoryID, Revision: rev.Revision, ContentSHA256: rev.ContentSHA256}
		j.BasisRefs = []BasisRef{{MemoryRef: &basis}}
	}
	j.ContentSHA256 = ""
	j = fillJudgmentHash(j)
	put(t, s, j)
	return j
}

func conflictReq(rev MemoryRevision, mc MemoryContext, store *FactStore) ConflictRequirementRequest {
	return ConflictRequirementRequest{
		Scope: rev.Scope, MemoryID: rev.MemoryID, Revision: rev.Revision,
		ExpectedMemoryContext: mc, ProjectStore: store, Now: criticReq(rev, mc).Now,
	}
}

func TestConflictRequirementMatrix(t *testing.T) {
	for i, tc := range []struct {
		name, result, evalScope string
		want                    ConflictRequirementStatus
	}{
		{"no review", "", "", ConflictRequirementUnavailable},
		{"full clear", "clear", "generation_full_scan", ConflictRequirementClear},
		{"sampled clear", "clear", "sampled_audit", ConflictRequirementUnavailable},
		{"conflict", "conflict", "generation_full_scan", ConflictRequirementUnresolved},
		{"unavailable", "unavailable", "generation_full_scan", ConflictRequirementUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s *FactStore
			var tx *GenerationTx
			var rev MemoryRevision
			var counterpart MemoryRef
			if tc.result == "conflict" {
				s, tx, rev, counterpart = conflictWorld(t, "conflict_matrix_"+string(rune('a'+i)))
			} else {
				_, s, tx, rev, _ = criticWorld(t, "conflict_matrix_"+string(rune('a'+i)))
			}
			mc := expectedContext(t, s, tx)
			var counterparts []MemoryRef
			if tc.result == "conflict" {
				counterparts = []MemoryRef{counterpart}
			}
			if tc.result != "" {
				putConflictReview(t, s, rev, mc, "judgment_conflict_matrix", tc.result, tc.evalScope, nil, counterparts)
			}
			got, err := EvaluateConflictRequirement(context.Background(), s, conflictReq(rev, mc, s))
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want || got.Satisfied != (tc.want == ConflictRequirementClear) {
				t.Fatalf("got %s satisfied=%v, want %s", got.Status, got.Satisfied, tc.want)
			}
		})
	}
}

func TestConflictRequirementClearSupersedesConflict(t *testing.T) {
	s, tx, rev, ref := conflictWorld(t, "conflict_supersede")
	mc := expectedContext(t, s, tx)
	old := putConflictReview(t, s, rev, mc, "judgment_conflict_old", "conflict", "generation_full_scan", nil, []MemoryRef{ref})
	sup := &JudgmentRef{Scope: old.Scope, JudgmentType: old.JudgmentType, JudgmentID: old.JudgmentID, ContentSHA256: old.ContentSHA256}
	putConflictReview(t, s, rev, mc, "judgment_conflict_clear", "clear", "generation_full_scan", sup, nil)
	got, err := EvaluateConflictRequirement(context.Background(), s, conflictReq(rev, mc, s))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ConflictRequirementClear || !got.Satisfied {
		t.Fatalf("superseding clear must satisfy, got %+v", got)
	}
}

func TestConflictRequirementCounterpartMustResolveExactly(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "conflict_missing_counterpart")
	mc := expectedContext(t, s, tx)
	missing := MemoryRef{Scope: ScopeProject, MemoryType: MemoryTypePattern, MemoryID: "mem_missing", Revision: 1, ContentSHA256: testHash}
	putConflictReview(t, s, rev, mc, "judgment_conflict_missing", "conflict", "generation_full_scan", nil, []MemoryRef{missing})
	if _, err := EvaluateConflictRequirement(context.Background(), s, conflictReq(rev, mc, s)); err == nil {
		t.Fatal("missing counterpart must fail closed")
	}
}

func TestConflictRequirementSubjectHashMismatchFailsClosed(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "conflict_subject_hash")
	mc := expectedContext(t, s, tx)
	j := validConflictJudgment(rev, "clear")
	j.JudgmentID = "judgment_conflict_bad_subject"
	j.ConflictReview.MemoryContext = mc
	j.Subject.MemoryRef.ContentSHA256 = testHash2
	j.ContentSHA256 = ""
	j = fillJudgmentHash(j)
	put(t, s, j)
	if _, err := EvaluateConflictRequirement(context.Background(), s, conflictReq(rev, mc, s)); ErrorCode(err) != CodeHashMismatch {
		t.Fatalf("subject hash mismatch must fail closed, got %v", err)
	}
}

func TestConflictRequirementParallelTerminalMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, second string
		want         ConflictRequirementStatus
	}{
		{"clear_and_conflict", "conflict", ConflictRequirementUnresolved},
		{"clear_and_unavailable", "unavailable", ConflictRequirementUnavailable},
		{"clear_and_clear", "clear", ConflictRequirementClear},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s *FactStore
			var tx *GenerationTx
			var rev MemoryRevision
			var counterpart MemoryRef
			if tc.second == "conflict" {
				s, tx, rev, counterpart = conflictWorld(t, "conflict_parallel_"+tc.second)
			} else {
				_, s, tx, rev, _ = criticWorld(t, "conflict_parallel_"+tc.second)
			}
			mc := expectedContext(t, s, tx)
			putConflictReview(t, s, rev, mc, "judgment_parallel_clear", "clear", "generation_full_scan", nil, nil)
			var counterparts []MemoryRef
			if tc.second == "conflict" {
				counterparts = []MemoryRef{counterpart}
			}
			putConflictReview(t, s, rev, mc, "judgment_parallel_second", tc.second, "generation_full_scan", nil, counterparts)
			got, err := EvaluateConflictRequirement(context.Background(), s, conflictReq(rev, mc, s))
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.want {
				t.Fatalf("got %s want %s", got.Status, tc.want)
			}
		})
	}
}

func TestConflictRequirementRejectsZeroNowAndFutureJudgment(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "conflict_time")
	mc := expectedContext(t, s, tx)
	req := conflictReq(rev, mc, s)
	req.Now = time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if _, err := EvaluateConflictRequirement(context.Background(), s, req); ErrorCode(err) != CodeEvaluationFutureReference {
		t.Fatalf("world later than Now must fail closed, got %v", err)
	}
	req = conflictReq(rev, mc, s)
	req.Now = time.Time{}
	if _, err := EvaluateConflictRequirement(context.Background(), s, req); ErrorCode(err) != CodeDerivedInvalidInput {
		t.Fatalf("zero Now must be rejected, got %v", err)
	}
}

func TestConflictRequirementReadOnly(t *testing.T) {
	_, s, tx, rev, _ := criticWorld(t, "conflict_readonly")
	mc := expectedContext(t, s, tx)
	putConflictReview(t, s, rev, mc, "judgment_conflict_readonly", "clear", "generation_full_scan", nil, nil)
	before := fileCount(t, s.root)
	if _, err := EvaluateConflictRequirement(context.Background(), s, conflictReq(rev, mc, s)); err != nil {
		t.Fatal(err)
	}
	if got := fileCount(t, s.root); got != before {
		t.Fatalf("conflict evaluation wrote files: before=%d after=%d", before, got)
	}
}
